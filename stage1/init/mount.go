// Copyright 2014 The rkt Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// file with functions that allow for creating a mount units for
// managing inner and external volumes in stage1
// mount units from systemd require to have /usr/bin/mount
// TODO: it's would better to just use host@.mount empty@.mount systemd templates, but
//		 technique requires 222 systemd
// TODO: move path creation to path.go
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/coreos/rkt/Godeps/_workspace/src/github.com/appc/spec/schema/types"
	"github.com/coreos/rkt/Godeps/_workspace/src/github.com/coreos/go-systemd/unit"
	"github.com/coreos/rkt/common"
)

const (
	stage1MntDir = "/mnt/" // in this place on stage1 rootfs shared volumes will be put (or empty directories for kind=empty)
)

// newMountUnit utility function that creates new mount
// unit in default systemd location (/usr/lib/systemd/system)
// root is an relative path pod stage1 filesystem e.g. uuid/rootfs/ Pod.Root
// requiredBy in install section creates required in given unit
func newMountUnit(root, what, where, fsType, options, requiredBy string) error {

	opts := []*unit.UnitOption{
		unit.NewUnitOption("Unit", "Description", fmt.Sprintf("Mount unit for %s", where)),
		unit.NewUnitOption("Unit", "DefaultDependencies", "false"),
		unit.NewUnitOption("Mount", "What", what),
		unit.NewUnitOption("Mount", "Where", where),
		unit.NewUnitOption("Mount", "Type", fsType),
		unit.NewUnitOption("Mount", "Options", options),
	}

	if requiredBy != "" {
		opts = append(opts, unit.NewUnitOption("Install", "RequiredBy", requiredBy))
	}

	unitsPath := filepath.Join(root, "/usr/lib/systemd/system")
	unitName := unit.UnitNamePathEscape(where + ".mount")

	file, err := os.OpenFile(filepath.Join(unitsPath, unitName), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to create service unit file %q: %v", unitName, err)
	}
	defer file.Close()

	if _, err = io.Copy(file, unit.Serialize(opts)); err != nil {
		return fmt.Errorf("failed to write service unit file %q: %v", unitName, err)
	}

	log.Printf("mount unit created: %q in %q (what=%q, where=%q)", unitName, unitsPath, what, where)
	return nil
}

// podToSystemdMountUnits create host shared remote file system mounts (using e.g. 9p)
// https://www.kernel.org/doc/Documentation/filesystems/9p.txt
// additionally create directories in some static path then bind mount them for each app
// root is stage1 root fs path for a given pod
func podToSystemdHostMountUnits(root string, volumes []types.Volume) error {

	// pod volumes need to mount p9 qemu mount_tags
	for _, vol := range volumes {
		// only host shared volumes

		name := vol.Name.String() // acts as a mount tag 9p
		// /var/lib/.../pod/run/rootfs/mnt/{volumeName}
		mountPoint := filepath.Join(root, stage1MntDir, name)

		// for vol.Kind we create an empty dir to be shared by applications
		log.Printf("creating an empty volume folder for sharing: %q", mountPoint)
		err := os.MkdirAll(mountPoint, 0700)
		if err != nil {
			return err
		}

		// for host kind we create a mount unit to mount host shared folder
		if vol.Kind == "host" {
			err = newMountUnit(root,
				name, // what (source) in 9p it is a channel tag which equals to volume.Name/mountPoint.name
				filepath.Join(stage1MntDir, name), // where - destination
				"9p",             // fsType
				"trans=virtio",   // 9p specific options
				"default.target", // required by default target // TODO: maybe require by any app ?
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// appToSystemdMountUnits prepare bind mount unit for empty or host kind mouting
// between stage1 rootfs and chrooted filesystem for application
// func (p *Pod) appToSystemdMountUnits(ra *schema.RuntimeApp) error {
func appToSystemdMountUnits(root string, appId types.Hash, mountPoints []types.MountPoint) error {

	for _, mountPoint := range mountPoints {

		name := mountPoint.Name.String()
		// source relative to stage1 rootfs to relative pod root
		whatPath := filepath.Join(stage1MntDir, name)
		whatFullPath := filepath.Join(root, whatPath)
		// destination relative to stage1 rootfs and relative to pod root
		wherePath := filepath.Join(common.RelAppRootfsPath(appId), mountPoint.Path)
		whereFullPath := filepath.Join(root, wherePath)

		// readOnly
		mountOptions := "bind"
		if mountPoint.ReadOnly {
			mountOptions += ",ro"
		}

		// make sure that "what" exists (created and mounted for pod)
		log.Printf("checking required source path: %q", whatFullPath)
		if _, err := os.Stat(whatFullPath); os.IsNotExist(err) {
			return fmt.Errorf("app requires a volume that is not defined in Pod (try --volume=%s,kind=empty)!", name)
		}

		// optionaly prepare app direcotry
		log.Printf("optionally preapring destination path: %q", whereFullPath)
		if _, err := os.Stat(whereFullPath); os.IsNotExist(err) {
			err := os.MkdirAll(whereFullPath, 0700)
			if err != nil {
				return fmt.Errorf("failed to prepare dir for mountPoint %v: %v", mountPoint.Name, err)
			}
		}

		// install new mount unit for bind mount /mnt/volumenName -> /opt/stage2/{app-id}/rootfs/{{mountPoint.Path}}
		err := newMountUnit(
			root,      // where put a mount unit
			whatPath,  // what - stage1 rootfs /mnt/VolumeName
			wherePath, // where - inside chroot app filesystem
			"bind",    // fstype
			mountOptions,
			ServiceUnitName(appId), // required by app.service
		)

		if err != nil {
			return err
		}

	}
	return nil
}

// PodToKvmArgs renders a prepared Pod as a lkvm
// argument list ready to be executed
// this arguments export pod volumes of kind host to quest
func PodToKvmDiskArgs(volumes []types.Volume) ([]string, error) {
	args := []string{}

	for _, vol := range volumes {
		mountTag := vol.Name.String() // tag/channel name for virtio
		if vol.Kind == "host" {
			// eg. --9p=/home/jon/srcdir,src
			arg := "--9p=" + vol.Source + "," + mountTag
			log.Printf("stage1: --disk argument: %#v\n", arg)
			args = append(args, arg)
		}
	}

	return args, nil
}
