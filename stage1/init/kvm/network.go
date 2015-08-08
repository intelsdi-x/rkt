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

package kvm

import (
	"fmt"
	"net"

	"github.com/coreos/rkt/networking"
)

// something that exposes description of network
type NetDescriber interface {
	HostIP() net.IP
	GuestIP() net.IP
	Mask() net.IP
	IfName() string
	IPMasq() bool
}

// GetKVMNetParams returns additional arguments that need to be passed to kernel
// and lkvm tool to configure networks properly.
// Logic is based on Network configuration extracted from Networking struct
// and essentially from activeNets that expose NetDescriber behavior
func GetKVMNetArgs(n *networking.Networking) ([]string, []string, error) {

	// convert activeNets to NetDescribers
	nds := []NetDescriber{}
	for _, an := range n.GetActiveNetworks() {
		nds = append(nds, an)
	}

	lkvmArgs := []string{}
	kernelParams := []string{}

	for i, nd := range nds {
		// https://www.kernel.org/doc/Documentation/filesystems/nfs/nfsroot.txt
		// ip=<client-ip>:<server-ip>:<gw-ip>:<netmask>:<hostname>:<device>:<autoconf>:<dns0-ip>:<dns1-ip>
		var gw string
		if nd.IPMasq() {
			gw = nd.HostIP().String()
		}
		kernelParams = append(kernelParams, "ip="+nd.GuestIP().String()+"::"+gw+":"+nd.Mask().String()+":"+fmt.Sprintf(networking.IfNamePattern, i)+":::")

		lkvmArgs = append(lkvmArgs, "--network", "mode=tap,tapif="+nd.IfName()+",host_ip="+nd.HostIP().String()+",guest_ip="+nd.GuestIP().String())
	}

	return lkvmArgs, kernelParams, nil
}
