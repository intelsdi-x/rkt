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

// +build host coreos src kvm

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/coreos/rkt/tests/testutils"
	"github.com/coreos/rkt/tests/testutils/logger"
	"github.com/vishvananda/netlink"
)

/*
 * Host network
 * ---
 * Container must have the same network namespace as the host
 */
func NewNetHostTest() testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		testImageArgs := []string{"--exec=/inspect --print-netns"}
		testImage := patchTestACI("rkt-inspect-networking.aci", testImageArgs...)
		defer os.Remove(testImage)

		ctx := testutils.NewRktRunCtx()
		defer ctx.Cleanup()

		cmd := fmt.Sprintf("%s --net=host --debug --insecure-options=image run --mds-register=false %s", ctx.Cmd(), testImage)
		child := spawnOrFail(t, cmd)
		ctx.RegisterChild(child)
		defer waitOrFail(t, child, 0)

		expectedRegex := `NetNS: (net:\[\d+\])`
		result, out, err := expectRegexWithOutput(child, expectedRegex)
		if err != nil {
			t.Fatalf("Error: %v\nOutput: %v", err, out)
		}

		ns, err := os.Readlink("/proc/self/ns/net")
		if err != nil {
			t.Fatalf("Cannot evaluate NetNS symlink: %v", err)
		}

		if nsChanged := ns != result[1]; nsChanged {
			t.Fatalf("container left host netns")
		}
	})
}

/*
 * Host networking
 * ---
 * Container launches http server which must be reachable by the host via the
 * localhost address
 */
func NewNetHostConnectivityTest() testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		logger.SetLogger(t)

		httpPort, err := testutils.GetNextFreePort4()
		if err != nil {
			t.Fatalf("%v", err)
		}
		httpServeAddr := fmt.Sprintf("0.0.0.0:%v", httpPort)
		httpGetAddr := fmt.Sprintf("http://127.0.0.1:%v", httpPort)

		testImageArgs := []string{"--exec=/inspect --serve-http=" + httpServeAddr}
		testImage := patchTestACI("rkt-inspect-networking.aci", testImageArgs...)
		defer os.Remove(testImage)

		ctx := testutils.NewRktRunCtx()
		defer ctx.Cleanup()

		cmd := fmt.Sprintf("%s --net=host --debug --insecure-options=image run --mds-register=false %s", ctx.Cmd(), testImage)
		child := spawnOrFail(t, cmd)
		ctx.RegisterChild(child)

		ga := testutils.NewGoroutineAssistant(t)
		ga.Add(2)

		// Child opens the server
		go func() {
			defer ga.Done()
			ga.WaitOrFail(child)
		}()

		// Host connects to the child
		go func() {
			defer ga.Done()
			expectedRegex := `serving on`
			_, out, err := expectRegexWithOutput(child, expectedRegex)
			if err != nil {
				ga.Fatalf("Error: %v\nOutput: %v", err, out)
			}
			body, err := testutils.HTTPGet(httpGetAddr)
			if err != nil {
				ga.Fatalf("%v\n", err)
			}
			t.Logf("HTTP-Get received: %s", body)
		}()

		ga.Wait()
	})
}

/*
 * None networking
 * ---
 * must be in an empty netns
 */
func NewNetNoneTest() testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		testImageArgs := []string{"--exec=/inspect --print-netns --print-iface-count"}
		testImage := patchTestACI("rkt-inspect-networking.aci", testImageArgs...)
		defer os.Remove(testImage)

		ctx := testutils.NewRktRunCtx()
		defer ctx.Cleanup()

		cmd := fmt.Sprintf("%s --debug --insecure-options=image run --net=none --mds-register=false %s", ctx.Cmd(), testImage)

		child := spawnOrFail(t, cmd)
		defer waitOrFail(t, child, 0)
		expectedRegex := `NetNS: (net:\[\d+\])`
		result, out, err := expectRegexWithOutput(child, expectedRegex)
		if err != nil {
			t.Fatalf("Error: %v\nOutput: %v", err, out)
		}

		ns, err := os.Readlink("/proc/self/ns/net")
		if err != nil {
			t.Fatalf("Cannot evaluate NetNS symlink: %v", err)
		}

		if nsChanged := ns != result[1]; !nsChanged {
			t.Fatalf("container did not leave host netns")
		}

		expectedRegex = `Interface count: (\d+)`
		result, out, err = expectRegexWithOutput(child, expectedRegex)
		if err != nil {
			t.Fatalf("Error: %v\nOutput: %v", err, out)
		}
		ifaceCount, err := strconv.Atoi(result[1])
		if err != nil {
			t.Fatalf("Error parsing interface count: %v\nOutput: %v", err, out)
		}
		if ifaceCount != 1 {
			t.Fatalf("Interface count must be 1 not %q", ifaceCount)
		}
	})
}

/*
 * Default net
 * ---
 * Container must be in a separate network namespace
 */
func NewTestNetDefaultNetNS() testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		testImageArgs := []string{"--exec=/inspect --print-netns"}
		testImage := patchTestACI("rkt-inspect-networking.aci", testImageArgs...)
		defer os.Remove(testImage)

		ctx := testutils.NewRktRunCtx()
		defer ctx.Cleanup()

		f := func(argument string) {
			cmd := fmt.Sprintf("%s --debug --insecure-options=image run %s --mds-register=false %s", ctx.Cmd(), argument, testImage)
			child := spawnOrFail(t, cmd)
			defer waitOrFail(t, child, 0)

			expectedRegex := `NetNS: (net:\[\d+\])`
			result, out, err := expectRegexWithOutput(child, expectedRegex)
			if err != nil {
				t.Fatalf("Error: %v\nOutput: %v", err, out)
			}

			ns, err := os.Readlink("/proc/self/ns/net")
			if err != nil {
				t.Fatalf("Cannot evaluate NetNS symlink: %v", err)
			}

			if nsChanged := ns != result[1]; !nsChanged {
				t.Fatalf("container did not leave host netns")
			}

		}
		f("--net=default")
		f("")
	})
}

/*
 * Default net
 * ---
 * Host launches http server on all interfaces in the host netns
 * Container must be able to connect via any IP address of the host in the
 * default network, which is NATed
 * TODO: test connection to host on an outside interface
 */
func NewNetDefaultConnectivityTest() testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		ctx := testutils.NewRktRunCtx()
		defer ctx.Cleanup()

		f := func(argument string) {
			httpPort, err := testutils.GetNextFreePort4()
			if err != nil {
				t.Fatalf("%v", err)
			}
			httpServeAddr := fmt.Sprintf("0.0.0.0:%v", httpPort)
			httpServeTimeout := 30

			nonLoIPv4, err := testutils.GetNonLoIfaceIPv4()
			if err != nil {
				t.Fatalf("%v", err)
			}
			if nonLoIPv4 == "" {
				t.Skipf("Can not find any NAT'able IPv4 on the host, skipping..")
			}

			httpGetAddr := fmt.Sprintf("http://%v:%v", nonLoIPv4, httpPort)
			t.Log("Telling the child to connect via", httpGetAddr)

			testImageArgs := []string{fmt.Sprintf("--exec=/inspect --get-http=%v", httpGetAddr)}
			testImage := patchTestACI("rkt-inspect-networking.aci", testImageArgs...)
			defer os.Remove(testImage)

			hostname, err := os.Hostname()
			if err != nil {
				t.Fatalf("Error getting hostname: %v", err)
			}

			ga := testutils.NewGoroutineAssistant(t)
			ga.Add(2)

			// Host opens the server
			go func() {
				defer ga.Done()
				err := testutils.HTTPServe(httpServeAddr, httpServeTimeout)
				if err != nil {
					ga.Fatalf("Error during HTTPServe: %v", err)
				}
			}()

			// Child connects to host
			go func() {
				defer ga.Done()
				cmd := fmt.Sprintf("%s --debug --insecure-options=image run %s --mds-register=false %s", ctx.Cmd(), argument, testImage)
				child := ga.SpawnOrFail(cmd)
				defer ga.WaitOrFail(child)

				expectedRegex := `HTTP-Get received: (.*)\r`
				result, out, err := expectRegexWithOutput(child, expectedRegex)
				if err != nil {
					ga.Fatalf("Error: %v\nOutput: %v", err, out)
				}
				if result[1] != hostname {
					ga.Fatalf("Hostname received by client `%v` doesn't match `%v`", result[1], hostname)
				}
			}()

			ga.Wait()
		}
		f("--net=default")
		f("")
	})
}

/*
 * Default-restricted net
 * ---
 * Container launches http server on all its interfaces
 * Host must be able to connects to container's http server via container's
 * eth0's IPv4
 * TODO: verify that the container isn't NATed
 */
func NewTestNetDefaultRestrictedConnectivity() testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		ctx := testutils.NewRktRunCtx()
		defer ctx.Cleanup()

		f := func(argument string) {
			httpPort, err := testutils.GetNextFreePort4()
			if err != nil {
				t.Fatalf("%v", err)
			}
			httpServeAddr := fmt.Sprintf("0.0.0.0:%v", httpPort)
			iface := "eth0"

			testImageArgs := []string{fmt.Sprintf("--exec=/inspect --print-ipv4=%v --serve-http=%v", iface, httpServeAddr)}
			testImage := patchTestACI("rkt-inspect-networking.aci", testImageArgs...)
			defer os.Remove(testImage)

			cmd := fmt.Sprintf("%s --debug --insecure-options=image run %s --mds-register=false %s", ctx.Cmd(), argument, testImage)
			child := spawnOrFail(t, cmd)

			expectedRegex := `IPv4: (\d+\.\d+\.\d+\.\d+)`
			result, out, err := expectRegexWithOutput(child, expectedRegex)
			if err != nil {
				t.Fatalf("Error: %v\nOutput: %v", err, out)
			}
			httpGetAddr := fmt.Sprintf("http://%v:%v", result[1], httpPort)

			ga := testutils.NewGoroutineAssistant(t)
			ga.Add(2)

			// Child opens the server
			go func() {
				defer ga.Done()
				ga.WaitOrFail(child)
			}()

			// Host connects to the child
			go func() {
				defer ga.Done()
				expectedRegex := `serving on`
				_, out, err := expectRegexWithOutput(child, expectedRegex)
				if err != nil {
					ga.Fatalf("Error: %v\nOutput: %v", err, out)
				}
				body, err := testutils.HTTPGet(httpGetAddr)
				if err != nil {
					ga.Fatalf("%v\n", err)
				}
				t.Logf("HTTP-Get received: %s", body)
			}()

			ga.Wait()
		}
		f("--net=default-restricted")
	})
}

type PortFwdCase struct {
	HttpGetIP     string
	HttpServePort int
	RktArg        string
	ShouldSucceed bool
}

var (
	bannedPorts = make(map[int]struct{}, 0)

	defaultSamePortFwdCase   = PortFwdCase{"172.16.28.1", 0, "--net=default", true}
	defaultDiffPortFwdCase   = PortFwdCase{"172.16.28.1", 1024, "--net=default", true}
	defaultLoSamePortFwdCase = PortFwdCase{"127.0.0.1", 0, "--net=default", true}
	defaultLoDiffPortFwdCase = PortFwdCase{"127.0.0.1", 1014, "--net=default", true}

	portFwdBridge = networkTemplateT{
		Name:      "bridge1",
		Type:      "bridge",
		Bridge:    "bridge1",
		IpMasq:    true,
		IsGateway: true,
		Ipam: &ipamTemplateT{
			Type:   "host-local",
			Subnet: "11.11.5.0/24",
			Routes: []map[string]string{
				{"dst": "0.0.0.0/0"},
			},
		},
	}
	bridgeSamePortFwdCase   = PortFwdCase{"11.11.5.1", 0, "--net=" + portFwdBridge.Name, true}
	bridgeDiffPortFwdCase   = PortFwdCase{"11.11.5.1", 1024, "--net=" + portFwdBridge.Name, true}
	bridgeLoSamePortFwdCase = PortFwdCase{"127.0.0.1", 0, "--net=" + portFwdBridge.Name, true}
	bridgeLoDiffPortFwdCase = PortFwdCase{"127.0.0.1", 1024, "--net=" + portFwdBridge.Name, true}
)

func (ct PortFwdCase) Execute(t *testing.T, ctx *testutils.RktRunCtx) {
	netdir := prepareTestNet(t, ctx, portFwdBridge)
	defer os.RemoveAll(netdir)

	httpPort, err := testutils.GetNextFreePort4Banned(bannedPorts)
	if err != nil {
		t.Fatalf("%v", err)
	}
	bannedPorts[httpPort] = struct{}{}

	httpServePort := ct.HttpServePort
	if httpServePort == 0 {
		httpServePort = httpPort
	}

	httpServeAddr := fmt.Sprintf("0.0.0.0:%d", httpServePort)
	testImageArgs := []string{
		fmt.Sprintf("--ports=http,protocol=tcp,port=%d", httpServePort),
		fmt.Sprintf("--exec=/inspect --serve-http=%v", httpServeAddr),
	}
	t.Logf("testImageArgs: %v", testImageArgs)
	testImage := patchTestACI("rkt-inspect-networking.aci", testImageArgs...)
	defer os.Remove(testImage)

	cmd := fmt.Sprintf(
		"%s --debug --insecure-options=image run --port=http:%d %s --mds-register=false %s",
		ctx.Cmd(), httpPort, ct.RktArg, testImage)
	child := spawnOrFail(t, cmd)

	httpGetAddr := fmt.Sprintf("http://%v:%v", ct.HttpGetIP, httpPort)

	ga := testutils.NewGoroutineAssistant(t)
	ga.Add(2)

	// Child opens the server
	go func() {
		defer ga.Done()
		ga.WaitOrFail(child)
	}()

	// Host connects to the child via the forward port on localhost
	go func() {
		defer ga.Done()
		expectedRegex := `serving on`
		_, out, err := expectRegexWithOutput(child, expectedRegex)
		if err != nil {
			ga.Fatalf("Error: %v\nOutput: %v", err, out)
		}
		body, err := testutils.HTTPGet(httpGetAddr)
		switch {
		case err != nil && ct.ShouldSucceed:
			ga.Fatalf("%v\n", err)
		case err == nil && !ct.ShouldSucceed:
			ga.Fatalf("HTTP-Get to %q should have failed! But received %q", httpGetAddr, body)
		case err != nil && !ct.ShouldSucceed:
			child.Close()
			fallthrough
		default:
			t.Logf("HTTP-Get received: %s", body)
		}
	}()

	ga.Wait()
}

type portFwdTest []PortFwdCase

func (ct portFwdTest) Execute(t *testing.T) {
	ctx := testutils.NewRktRunCtx()
	defer ctx.Cleanup()

	for _, testCase := range ct {
		testCase.Execute(t, ctx)
	}
}

/*
 * Net port forwarding connectivity
 * ---
 * Container launches http server on all its interfaces
 * Host must be able to connect to container's http server on it's own interfaces
 */
func NewNetPortFwdConnectivityTest(cases ...PortFwdCase) testutils.Test {
	return portFwdTest(cases)
}

func writeNetwork(t *testing.T, net networkTemplateT, netd string) error {
	var err error
	path := filepath.Join(netd, net.Name+".conf")
	file, err := os.Create(path)
	if err != nil {
		t.Errorf("%v", err)
	}

	b, err := json.Marshal(net)
	if err != nil {
		return err
	}

	fmt.Println("Writing", net.Name, "to", path)
	_, err = file.Write(b)
	if err != nil {
		return err
	}

	return nil
}

type networkTemplateT struct {
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	SubnetFile string             `json:"subnetFile,omitempty"`
	Master     string             `json:"master,omitempty"`
	IpMasq     bool               `json:",omitempty"`
	IsGateway  bool               `json:",omitempty"`
	Bridge     string             `json:"bridge,omitempty"`
	Ipam       *ipamTemplateT     `json:",omitempty"`
	Delegate   *delegateTemplateT `json:",omitempty"`
}

type ipamTemplateT struct {
	Type   string              `json:",omitempty"`
	Subnet string              `json:"subnet,omitempty"`
	Routes []map[string]string `json:"routes,omitempty"`
}

type delegateTemplateT struct {
	Bridge           string `json:"bridge,omitempty"`
	IsDefaultGateway bool   `json:"isDefaultGateway"`
}

func TestNetTemplates(t *testing.T) {
	net := networkTemplateT{
		Name: "ptp0",
		Type: "ptp",
		Ipam: &ipamTemplateT{
			Type:   "host-local",
			Subnet: "11.11.3.0/24",
			Routes: []map[string]string{{"dst": "0.0.0.0/0"}},
		},
	}

	b, err := json.Marshal(net)
	if err != nil {
		t.Fatalf("%v", err)
	}
	expected := `{"Name":"ptp0","Type":"ptp","IpMasq":false,"IsGateway":false,"Ipam":{"Type":"host-local","subnet":"11.11.3.0/24","routes":[{"dst":"0.0.0.0/0"}]}}`
	if string(b) != expected {
		t.Fatalf("Template extected:\n%v\ngot:\n%v\n", expected, string(b))
	}
}

func prepareTestNet(t *testing.T, ctx *testutils.RktRunCtx, nt networkTemplateT) (netdir string) {
	configdir := ctx.LocalDir()
	netdir = filepath.Join(configdir, "net.d")
	err := os.MkdirAll(netdir, 0644)
	if err != nil {
		t.Fatalf("Cannot create netdir: %v", err)
	}
	err = writeNetwork(t, nt, netdir)
	if err != nil {
		t.Fatalf("Cannot write network file: %v", err)
	}
	return netdir
}

/*
 * Two containers spawn in the same custom network.
 * ---
 * Container 1 opens the http server
 * Container 2 fires a HTTPGet on it
 * The body of the HTTPGet is Container 1's hostname, which must match
 */
func testNetCustomDual(t *testing.T, nt networkTemplateT) {
	httpPort, err := testutils.GetNextFreePort4()
	if err != nil {
		t.Fatalf("%v", err)
	}

	ctx := testutils.NewRktRunCtx()
	defer ctx.Cleanup()

	netdir := prepareTestNet(t, ctx, nt)
	defer os.RemoveAll(netdir)

	container1IPv4, container1Hostname := make(chan string), make(chan string)
	ga := testutils.NewGoroutineAssistant(t)
	ga.Add(2)

	go func() {
		defer ga.Done()
		httpServeAddr := fmt.Sprintf("0.0.0.0:%v", httpPort)
		testImageArgs := []string{"--exec=/inspect --print-ipv4=eth0 --serve-http=" + httpServeAddr}
		testImage := patchTestACI("rkt-inspect-networking1.aci", testImageArgs...)
		defer os.Remove(testImage)

		cmd := fmt.Sprintf("%s --debug --insecure-options=image run --net=%v --mds-register=false %s", ctx.Cmd(), nt.Name, testImage)
		child := ga.SpawnOrFail(cmd)
		defer ga.WaitOrFail(child)

		expectedRegex := `IPv4: (\d+\.\d+\.\d+\.\d+)`
		result, out, err := expectRegexTimeoutWithOutput(child, expectedRegex, 30*time.Second)
		if err != nil {
			ga.Fatalf("Error: %v\nOutput: %v", err, out)
		}
		container1IPv4 <- result[1]
		expectedRegex = ` ([a-zA-Z0-9\-]*): serving on`
		result, out, err = expectRegexTimeoutWithOutput(child, expectedRegex, 30*time.Second)
		if err != nil {
			ga.Fatalf("Error: %v\nOutput: %v", err, out)
		}
		container1Hostname <- result[1]
	}()

	go func() {
		defer ga.Done()

		var httpGetAddr string
		httpGetAddr = fmt.Sprintf("http://%v:%v", <-container1IPv4, httpPort)

		testImageArgs := []string{"--exec=/inspect --get-http=" + httpGetAddr}
		testImage := patchTestACI("rkt-inspect-networking2.aci", testImageArgs...)
		defer os.Remove(testImage)

		cmd := fmt.Sprintf("%s --debug --insecure-options=image run --net=%v --mds-register=false %s", ctx.Cmd(), nt.Name, testImage)
		child := ga.SpawnOrFail(cmd)
		defer ga.WaitOrFail(child)

		expectedHostname := <-container1Hostname
		expectedRegex := `HTTP-Get received: (.*?)\r`
		result, out, err := expectRegexTimeoutWithOutput(child, expectedRegex, 20*time.Second)
		if err != nil {
			ga.Fatalf("Error: %v\nOutput: %v", err, out)
		}
		t.Logf("HTTP-Get received: %s", result[1])
		receivedHostname := result[1]

		if receivedHostname != expectedHostname {
			ga.Fatalf("Received hostname `%v` doesn't match `%v`", receivedHostname, expectedHostname)
		}
	}()

	ga.Wait()
}

/*
 * Host launches http server on all interfaces in the host netns
 * Container must be able to connect via any IP address of the host in the
 * macvlan network, which is NAT
 * TODO: test connection to host on an outside interface
 */
func testNetCustomNatConnectivity(t *testing.T, nt networkTemplateT) {
	ctx := testutils.NewRktRunCtx()
	defer ctx.Cleanup()

	netdir := prepareTestNet(t, ctx, nt)
	defer os.RemoveAll(netdir)

	httpPort, err := testutils.GetNextFreePort4()
	if err != nil {
		t.Fatalf("%v", err)
	}
	httpServeAddr := fmt.Sprintf("0.0.0.0:%v", httpPort)
	httpServeTimeout := 30

	nonLoIPv4, err := testutils.GetNonLoIfaceIPv4()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if nonLoIPv4 == "" {
		t.Skipf("Can not find any NAT'able IPv4 on the host, skipping..")
	}

	httpGetAddr := fmt.Sprintf("http://%v:%v", nonLoIPv4, httpPort)
	t.Log("Telling the child to connect via", httpGetAddr)

	ga := testutils.NewGoroutineAssistant(t)
	ga.Add(2)

	// Host opens the server
	go func() {
		defer ga.Done()
		err := testutils.HTTPServe(httpServeAddr, httpServeTimeout)
		if err != nil {
			ga.Fatalf("Error during HTTPServe: %v", err)
		}
	}()

	// Child connects to host
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	go func() {
		defer ga.Done()
		testImageArgs := []string{fmt.Sprintf("--exec=/inspect --get-http=%v", httpGetAddr)}
		testImage := patchTestACI("rkt-inspect-networking.aci", testImageArgs...)
		defer os.Remove(testImage)

		cmd := fmt.Sprintf("%s --debug --insecure-options=image run --net=%v --mds-register=false %s", ctx.Cmd(), nt.Name, testImage)
		child := ga.SpawnOrFail(cmd)
		defer ga.WaitOrFail(child)

		expectedRegex := `HTTP-Get received: (.*?)\r`
		result, out, err := expectRegexWithOutput(child, expectedRegex)
		if err != nil {
			ga.Fatalf("Error: %v\nOutput: %v", err, out)
		}

		if result[1] != hostname {
			ga.Fatalf("Hostname received by client `%v` doesn't match `%v`", result[1], hostname)
		}
	}()

	ga.Wait()
}

func NewNetCustomPtpTest(runCustomDual bool) testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		nt := networkTemplateT{
			Name:   "ptp0",
			Type:   "ptp",
			IpMasq: true,
			Ipam: &ipamTemplateT{
				Type:   "host-local",
				Subnet: "11.11.1.0/24",
				Routes: []map[string]string{
					{"dst": "0.0.0.0/0"},
				},
			},
		}
		testNetCustomNatConnectivity(t, nt)
		if runCustomDual {
			testNetCustomDual(t, nt)
		}
	})
}

func NewNetCustomMacvlanTest() testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		iface, _, err := testutils.GetNonLoIfaceWithAddrs(netlink.FAMILY_V4)
		if err != nil {
			t.Fatalf("Error while getting non-lo host interface: %v\n", err)
		}
		if iface.Name == "" {
			t.Skipf("Cannot run test without non-lo host interface")
		}

		nt := networkTemplateT{
			Name:   "macvlan0",
			Type:   "macvlan",
			Master: iface.Name,
			Ipam: &ipamTemplateT{
				Type:   "host-local",
				Subnet: "11.11.2.0/24",
			},
		}
		testNetCustomDual(t, nt)
	})
}

func NewNetCustomBridgeTest() testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		iface, _, err := testutils.GetNonLoIfaceWithAddrs(netlink.FAMILY_V4)
		if err != nil {
			t.Fatalf("Error while getting non-lo host interface: %v\n", err)
		}
		if iface.Name == "" {
			t.Skipf("Cannot run test without non-lo host interface")
		}

		nt := networkTemplateT{
			Name:      "bridge0",
			Type:      "bridge",
			IpMasq:    true,
			IsGateway: true,
			Master:    iface.Name,
			Ipam: &ipamTemplateT{
				Type:   "host-local",
				Subnet: "11.11.3.0/24",
				Routes: []map[string]string{
					{"dst": "0.0.0.0/0"},
				},
			},
		}
		testNetCustomNatConnectivity(t, nt)
		testNetCustomDual(t, nt)
	})
}

func NewNetOverrideTest() testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		ctx := testutils.NewRktRunCtx()
		defer ctx.Cleanup()

		iface, _, err := testutils.GetNonLoIfaceWithAddrs(netlink.FAMILY_V4)
		if err != nil {
			t.Fatalf("Error while getting non-lo host interface: %v\n", err)
		}
		if iface.Name == "" {
			t.Skipf("Cannot run test without non-lo host interface")
		}

		nt := networkTemplateT{
			Name:   "overridemacvlan",
			Type:   "macvlan",
			Master: iface.Name,
			Ipam: &ipamTemplateT{
				Type:   "host-local",
				Subnet: "11.11.4.0/24",
			},
		}

		netdir := prepareTestNet(t, ctx, nt)
		defer os.RemoveAll(netdir)

		testImageArgs := []string{"--exec=/inspect --print-ipv4=eth0"}
		testImage := patchTestACI("rkt-inspect-networking1.aci", testImageArgs...)
		defer os.Remove(testImage)

		expectedIP := "11.11.4.244"

		cmd := fmt.Sprintf("%s --debug --insecure-options=image run --net=all --net=\"%s:IP=%s\" --mds-register=false %s", ctx.Cmd(), nt.Name, expectedIP, testImage)
		child := spawnOrFail(t, cmd)
		defer waitOrFail(t, child, 0)

		expectedRegex := `IPv4: (\d+\.\d+\.\d+\.\d+)`
		result, out, err := expectRegexTimeoutWithOutput(child, expectedRegex, 30*time.Second)
		if err != nil {
			t.Fatalf("Error: %v\nOutput: %v", err, out)
			return
		}

		containerIP := result[1]
		if expectedIP != containerIP {
			t.Fatalf("overriding IP did not work: Got %q but expected %q", containerIP, expectedIP)
		}
	})
}

func NewTestNetLongName() testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		nt := networkTemplateT{
			Name:   "thisnameiswaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaytoolong",
			Type:   "ptp",
			IpMasq: true,
			Ipam: &ipamTemplateT{
				Type:   "host-local",
				Subnet: "11.11.6.0/24",
				Routes: []map[string]string{
					{"dst": "0.0.0.0/0"},
				},
			},
		}
		testNetCustomNatConnectivity(t, nt)
	})
}

func mockFlannelNetwork(t *testing.T, ctx *testutils.RktRunCtx) (string, networkTemplateT, error) {
	// write fake flannel info
	subnetPath := filepath.Join(ctx.DataDir(), "subnet.env")
	file, err := os.Create(subnetPath)
	if err != nil {
		return "", networkTemplateT{}, err
	}
	mockedFlannel := fmt.Sprintf("FLANNEL_NETWORK=10.1.0.0/16\nFLANNEL_SUBNET=10.1.17.1/24\nFLANNEL_MTU=1472\nFLANNEL_IPMASQ=true\n")
	_, err = file.WriteString(mockedFlannel)
	if err != nil {
		return "", networkTemplateT{}, err
	}

	_ = file.Close()

	// write net config for "flannel" based network
	ntFlannel := networkTemplateT{
		Name:       "rkt.kubernetes.io",
		Type:       "flannel",
		SubnetFile: subnetPath,
		Delegate: &delegateTemplateT{
			IsDefaultGateway: true,
		},
	}

	netdir := prepareTestNet(t, ctx, ntFlannel)

	return netdir, ntFlannel, nil
}

/*
 * Check if netName is set if network is configured via flannel
 */
func NewNetPreserveNetNameTest() testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		ctx := testutils.NewRktRunCtx()
		defer ctx.Cleanup()

		netdir, ntFlannel, err := mockFlannelNetwork(t, ctx)
		if err != nil {
			t.Errorf("Can't mock flannel network: %v", err)
		}

		defer os.RemoveAll(netdir)
		defer os.Remove(ntFlannel.SubnetFile)

		runList, listOutput := make(chan bool), make(chan bool)
		ga := testutils.NewGoroutineAssistant(t)
		ga.Add(2)

		go func() {
			// busybox with mock'd flannel network
			defer ga.Done()
			cmd := fmt.Sprintf("%s --debug --insecure-options=image run --net=%v --mds-register=false docker://busybox --exec /bin/sh -- -c \"echo 'sleeping' && sleep 120 \"", ctx.Cmd(), ntFlannel.Name)
			child := ga.SpawnOrFail(cmd)

			_, _, err := expectRegexTimeoutWithOutput(child, "sleeping", 30*time.Second)
			if err != nil {
				t.Fatal("Can't spawn container")
			}
			runList <- true
			<-runList

			ga.WaitOrFail(child)
		}()

		go func() {
			// rkt list
			defer ga.Done()
			<-runList
			cmd := fmt.Sprintf("%s list", ctx.Cmd())
			child := ga.SpawnOrFail(cmd)

			_, _, err := expectRegexTimeoutWithOutput(child, "rkt.kubernetes.io", 30*time.Second)
			if err != nil {
				listOutput <- false
			} else {
				listOutput <- true
			}
			runList <- true

			ga.WaitOrFail(child)
		}()

		netFound := <-listOutput
		if !netFound {
			t.Fatal("netName not set")
		}
	})
}

func NewNetDefaultGWTest() testutils.Test {
	return testutils.TestFunc(func(t *testing.T) {
		ctx := testutils.NewRktRunCtx()
		defer ctx.Cleanup()

		netdir, ntFlannel, err := mockFlannelNetwork(t, ctx)
		if err != nil {
			t.Errorf("Can't mock flannel network: %v", err)
		}

		defer os.RemoveAll(netdir)
		defer os.Remove(ntFlannel.SubnetFile)

		ga := testutils.NewGoroutineAssistant(t)
		ga.Add(2)

		testImageArgs := []string{"--exec=/inspect --print-netns"}
		testImage := patchTestACI("rkt-inspect-networking1.aci", testImageArgs...)
		defer os.Remove(testImage)

		cmd := fmt.Sprintf("%s --debug --insecure-options=image run --net=%s --mds-register=false docker://busybox --exec ip -- route", ctx.Cmd(), ntFlannel.Name)
		child := spawnOrFail(t, cmd)
		defer waitOrFail(t, child, 0)

		expectedRegex := `default via (\d+\.\d+\.\d+\.\d+)`
		result, out, err := expectRegexTimeoutWithOutput(child, expectedRegex, 30*time.Second)
		if err != nil {
			t.Fatalf("Result: %v\nError: %v\nOutput: %v", result, err, out)
		}
	})
}
