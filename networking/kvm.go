// Copyright 2015 The rkt Authors
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

// kvm.go file provides networking supporting functions for kvm flavor
package networking

import (
	"fmt"
	"net"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/vishvananda/netlink"
)

const (
	defaultBrName     = "kvm-cni0"
	defaultSubnetFile = "/run/flannel/subnet.env"
	defaultMTU        = 1500
)

type BridgeNetConf struct {
	NetConf
	BrName string `json:"bridge"`
	IsGw   bool   `json:"isGateway"`
}

// Following methods implements behavior of netDescriber by activeNet
// (behavior required by stage1/init/kvm package and its kernel parameters configuration)

func (an activeNet) HostIP() net.IP {
	return an.runtime.IP4.Gateway
}
func (an activeNet) GuestIP() net.IP {
	return an.runtime.IP4.IP.IP
}
func (an activeNet) IfName() string {
	if an.conf.Type == "macvlan" {
		// macvtap device passed as parameter to lkvm binary have different
		// kind of name, path to /dev/tapN made with N as link index
		link, err := netlink.LinkByName(an.runtime.IfName)
		if err != nil {
			stderr.PrintE(fmt.Sprintf("cannot get interface '%v'", an.runtime.IfName), err)
			return ""
		}
		return fmt.Sprintf("/dev/tap%d", link.Attrs().Index)
	}
	return an.runtime.IfName
}
func (an activeNet) Mask() net.IPMask {
	return an.runtime.IP4.IP.Mask
}
func (an activeNet) Name() string {
	return an.conf.Name
}
func (an activeNet) IPMasq() bool {
	return an.conf.IPMasq
}
func (an activeNet) Gateway() net.IP {
	return an.runtime.IP4.Gateway
}
func (an activeNet) Routes() []cnitypes.Route {
	return an.runtime.IP4.Routes
}

func (an activeNet) IPC() *cnitypes.IPConfig {
	return an.runtime.IP4
}

// GetActiveNetworks returns activeNets to be used as NetDescriptors
// by plugins, which are required for stage1 executor to run (only for KVM)
func (e *Networking) GetActiveNetworks() []activeNet {
	return e.nets
}
