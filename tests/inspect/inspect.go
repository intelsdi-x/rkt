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

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/coreos/rkt/Godeps/_workspace/src/github.com/syndtr/gocapability/capability"
	"github.com/coreos/rkt/common/cgroup"
	"github.com/coreos/rkt/tests/testutils"
)

var (
	globalFlagset = flag.NewFlagSet("inspect", flag.ExitOnError)
	globalFlags   = struct {
		ReadStdin         bool
		CheckTty          bool
		PrintExec         bool
		PrintMsg          string
		PrintEnv          string
		PrintCapsPid      int
		PrintUser         bool
		CheckCwd          string
		ExitCode          int
		ReadFile          bool
		WriteFile         bool
		Sleep             int
		PreSleep          int
		PrintMemoryLimit  bool
		PrintCPUQuota     bool
		FileName          string
		Content           string
		CheckCgroupMounts bool
		PrintNetNS        bool
		PrintIPv4         string
		PrintIPv6         string
		PrintDefaultGWv4  bool
		PrintDefaultGWv6  bool
		PrintGWv4         string
		PrintGWv6         string
		GetHttp           string
		ServeHttp         string
		ServeHttpTimeout  int
		PrintIfUnsecure   bool
	}{}
)

func init() {
	globalFlagset.BoolVar(&globalFlags.ReadStdin, "read-stdin", false, "Read a line from stdin")
	globalFlagset.BoolVar(&globalFlags.CheckTty, "check-tty", false, "Check if stdin is a terminal")
	globalFlagset.BoolVar(&globalFlags.PrintExec, "print-exec", false, "Print the command we were execed as (i.e. argv[0])")
	globalFlagset.StringVar(&globalFlags.PrintMsg, "print-msg", "", "Print the message given as parameter")
	globalFlagset.StringVar(&globalFlags.CheckCwd, "check-cwd", "", "Check if the current working directory is the one specified")
	globalFlagset.StringVar(&globalFlags.PrintEnv, "print-env", "", "Print the specified environment variable")
	globalFlagset.IntVar(&globalFlags.PrintCapsPid, "print-caps-pid", -1, "Print capabilities of the specified pid (or current process if pid=0)")
	globalFlagset.BoolVar(&globalFlags.PrintUser, "print-user", false, "Print uid and gid")
	globalFlagset.IntVar(&globalFlags.ExitCode, "exit-code", 0, "Return this exit code")
	globalFlagset.BoolVar(&globalFlags.ReadFile, "read-file", false, "Print the content of the file $FILE")
	globalFlagset.BoolVar(&globalFlags.WriteFile, "write-file", false, "Write $CONTENT in the file $FILE")
	globalFlagset.IntVar(&globalFlags.Sleep, "sleep", -1, "Sleep before exiting (in seconds)")
	globalFlagset.IntVar(&globalFlags.PreSleep, "pre-sleep", -1, "Sleep before executing (in seconds)")
	globalFlagset.BoolVar(&globalFlags.PrintMemoryLimit, "print-memorylimit", false, "Print cgroup memory limit")
	globalFlagset.BoolVar(&globalFlags.PrintCPUQuota, "print-cpuquota", false, "Print cgroup cpu quota in milli-cores")
	globalFlagset.StringVar(&globalFlags.FileName, "file-name", "", "The file to read/write, $FILE will be ignored if this is specified")
	globalFlagset.StringVar(&globalFlags.Content, "content", "", "The content to write, $CONTENT will be ignored if this is specified")
	globalFlagset.BoolVar(&globalFlags.CheckCgroupMounts, "check-cgroups", false, "Try to write to the cgroup filesystem. Everything should be RO except some well-known files")
	globalFlagset.BoolVar(&globalFlags.PrintNetNS, "print-netns", false, "Print the network namespace")
	globalFlagset.StringVar(&globalFlags.PrintIPv4, "print-ipv4", "", "Takes an interface name and prints its IPv4")
	globalFlagset.StringVar(&globalFlags.PrintIPv6, "print-ipv6", "", "Takes an interface name and prints its IPv6")
	globalFlagset.BoolVar(&globalFlags.PrintDefaultGWv4, "print-defaultgwv4", false, "Print the default IPv4 gateway")
	globalFlagset.BoolVar(&globalFlags.PrintDefaultGWv6, "print-defaultgwv6", false, "Print the default IPv6 gateway")
	globalFlagset.StringVar(&globalFlags.PrintGWv4, "print-gwv4", "", "Takes an interface name and prints its gateway's IPv4")
	globalFlagset.StringVar(&globalFlags.PrintGWv6, "print-gwv6", "", "Takes an interface name and prints its gateway's IPv6")
	globalFlagset.StringVar(&globalFlags.GetHttp, "get-http", "", "HTTP-Get from the given address")
	globalFlagset.StringVar(&globalFlags.ServeHttp, "serve-http", "", "Serve the hostname via HTTP on the given address:port")
	globalFlagset.IntVar(&globalFlags.ServeHttpTimeout, "serve-http-timeout", 30, "HTTP Timeout to wait for a client connection")
	globalFlagset.BoolVar(&globalFlags.PrintIfUnsecure, "print-if-unsecure", false, "Print if cgroups are changeable")
}

func main() {
	globalFlagset.Parse(os.Args[1:])
	args := globalFlagset.Args()
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "Wrong parameters")
		os.Exit(1)
	}

	if globalFlags.PreSleep >= 0 {
		time.Sleep(time.Duration(globalFlags.PreSleep) * time.Second)
	}

	if globalFlags.ReadStdin {
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("Enter text:\n")
		text, _ := reader.ReadString('\n')
		fmt.Printf("Received text: %s\n", text)
	}

	if globalFlags.CheckTty {
		fd := int(os.Stdin.Fd())
		var termios syscall.Termios
		_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), syscall.TCGETS, uintptr(unsafe.Pointer(&termios)), 0, 0, 0)
		if err == 0 {
			fmt.Printf("stdin is a terminal\n")
		} else {
			fmt.Printf("stdin is not a terminal\n")
		}
	}

	if globalFlags.PrintExec {
		fmt.Fprintf(os.Stdout, "inspect execed as: %s\n", os.Args[0])
	}

	if globalFlags.PrintMsg != "" {
		fmt.Fprintf(os.Stdout, "%s\n", globalFlags.PrintMsg)
		messageLoopStr := os.Getenv("MESSAGE_LOOP")
		messageLoop, err := strconv.Atoi(messageLoopStr)
		if err == nil {
			for i := 0; i < messageLoop; i++ {
				time.Sleep(time.Second)
				fmt.Fprintf(os.Stdout, "%s\n", globalFlags.PrintMsg)
			}
		}
	}

	if globalFlags.PrintEnv != "" {
		fmt.Fprintf(os.Stdout, "%s=%s\n", globalFlags.PrintEnv, os.Getenv(globalFlags.PrintEnv))
	}

	if globalFlags.PrintCapsPid >= 0 {
		caps, err := capability.NewPid(globalFlags.PrintCapsPid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot get caps: %v\n", err)
			os.Exit(1)
			return
		}
		fmt.Printf("Capability set: effective: %s\n", caps.StringCap(capability.EFFECTIVE))
		fmt.Printf("Capability set: permitted: %s\n", caps.StringCap(capability.PERMITTED))
		fmt.Printf("Capability set: inheritable: %s\n", caps.StringCap(capability.INHERITABLE))
		fmt.Printf("Capability set: bounding: %s\n", caps.StringCap(capability.BOUNDING))

		if capStr := os.Getenv("CAPABILITY"); capStr != "" {
			capInt, err := strconv.Atoi(capStr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Environment variable $CAPABILITY is not a valid capability number: %v\n", err)
				os.Exit(1)
				return
			}
			c := capability.Cap(capInt)
			if caps.Get(capability.BOUNDING, c) {
				fmt.Printf("%v=enabled\n", c.String())
			} else {
				fmt.Printf("%v=disabled\n", c.String())
			}
		}
	}

	if globalFlags.PrintUser {
		fmt.Printf("User: uid=%d euid=%d gid=%d egid=%d\n", os.Getuid(), os.Geteuid(), os.Getgid(), os.Getegid())
	}

	if globalFlags.WriteFile {
		fileName := os.Getenv("FILE")
		if globalFlags.FileName != "" {
			fileName = globalFlags.FileName
		}
		content := os.Getenv("CONTENT")
		if globalFlags.Content != "" {
			content = globalFlags.Content
		}

		err := ioutil.WriteFile(fileName, []byte(content), 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot write to file %q: %v\n", fileName, err)
			os.Exit(1)
			return
		}
	}

	if globalFlags.ReadFile {
		fileName := os.Getenv("FILE")
		if globalFlags.FileName != "" {
			fileName = globalFlags.FileName
		}

		dat, err := ioutil.ReadFile(fileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot read file %q: %v\n", fileName, err)
			os.Exit(1)
			return
		}
		fmt.Print("<<<")
		fmt.Print(string(dat))
		fmt.Print(">>>\n")
	}

	if globalFlags.CheckCwd != "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot get working directory: %v\n", err)
			os.Exit(1)
		}
		if wd != globalFlags.CheckCwd {
			fmt.Fprintf(os.Stderr, "Working directory: %q. Expected: %q.\n", wd, globalFlags.CheckCwd)
			os.Exit(1)
		}
	}

	if globalFlags.Sleep >= 0 {
		time.Sleep(time.Duration(globalFlags.Sleep) * time.Second)
	}

	if globalFlags.PrintMemoryLimit {
		memCgroupPath, err := cgroup.GetOwnCgroupPath("memory")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting own memory cgroup path: %v\n", err)
			os.Exit(1)
		}
		// we use /proc/1/root to escape the chroot we're in and read our
		// memory limit
		limitPath := filepath.Join("/proc/1/root/sys/fs/cgroup/memory", memCgroupPath, "memory.limit_in_bytes")
		limit, err := ioutil.ReadFile(limitPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can't read memory.limit_in_bytes\n")
			os.Exit(1)
		}

		fmt.Printf("Memory Limit: %s\n", string(limit))
	}

	if globalFlags.PrintCPUQuota {
		cpuCgroupPath, err := cgroup.GetOwnCgroupPath("cpu")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting own cpu cgroup path: %v\n", err)
			os.Exit(1)
		}
		// we use /proc/1/root to escape the chroot we're in and read our
		// cpu quota
		periodPath := filepath.Join("/proc/1/root/sys/fs/cgroup/cpu", cpuCgroupPath, "cpu.cfs_period_us")
		periodBytes, err := ioutil.ReadFile(periodPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can't read cpu.cpu_period_us\n")
			os.Exit(1)
		}
		quotaPath := filepath.Join("/proc/1/root/sys/fs/cgroup/cpu", cpuCgroupPath, "cpu.cfs_quota_us")
		quotaBytes, err := ioutil.ReadFile(quotaPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can't read cpu.cpu_quota_us\n")
			os.Exit(1)
		}

		period, err := strconv.Atoi(strings.Trim(string(periodBytes), "\n"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		quota, err := strconv.Atoi(strings.Trim(string(quotaBytes), "\n"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		quotaMilliCores := quota * 1000 / period
		fmt.Printf("CPU Quota: %s\n", strconv.Itoa(quotaMilliCores))
	}

	if globalFlags.CheckCgroupMounts {
		rootCgroupPath := "/proc/1/root/sys/fs/cgroup"
		testPaths := []string{rootCgroupPath}

		// test a couple of controllers if they're available
		if cgroup.IsIsolatorSupported("memory") {
			testPaths = append(testPaths, filepath.Join(rootCgroupPath, "memory"))
		}
		if cgroup.IsIsolatorSupported("cpu") {
			testPaths = append(testPaths, filepath.Join(rootCgroupPath, "cpu"))
		}

		for _, p := range testPaths {
			if err := syscall.Mkdir(filepath.Join(p, "test"), 0600); err == nil || err != syscall.EROFS {
				fmt.Println("check-cgroups: FAIL")
				os.Exit(1)
			}
		}

		fmt.Println("check-cgroups: SUCCESS")
	}

	if globalFlags.PrintNetNS {
		ns, err := os.Readlink("/proc/self/ns/net")
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("NetNS: %s\n", ns)
	}

	if globalFlags.PrintIPv4 != "" {
		iface := globalFlags.PrintIPv4
		ips, err := testutils.GetIPsv4(iface)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("%v IPv4: %s\n", iface, ips[0])
	}

	if globalFlags.PrintDefaultGWv4 {
		gw, err := testutils.GetDefaultGWv4()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("DefaultGWv4: %s\n", gw)
	}

	if globalFlags.PrintDefaultGWv6 {
		gw, err := testutils.GetDefaultGWv6()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("DefaultGWv6: %s\n", gw)
	}

	if globalFlags.PrintGWv4 != "" {
		// TODO: GetGW not implemented yet
		iface := globalFlags.PrintGWv4
		gw, err := testutils.GetGWv4(iface)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("%v GWv4: %s\n", iface, gw)
	}

	if globalFlags.PrintIPv6 != "" {
		// TODO
	}

	if globalFlags.PrintGWv6 != "" {
		// TODO
	}

	if globalFlags.ServeHttp != "" {
		err := testutils.HttpServe(globalFlags.ServeHttp, globalFlags.ServeHttpTimeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}

	if globalFlags.GetHttp != "" {
		body, err := testutils.HttpGet(globalFlags.GetHttp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("HTTP-Get received: %s\n", body)
	}

	if globalFlags.PrintIfUnsecure {
		//cpu subsystem mount point
		cpu_mp := "sys/fs/cgroup/cpu"

		//Make tmp dit
		_ = os.Mkdir("/jail", 0777)
		//Break jail
		_ = os.Chdir("/")
		//JAILROOT
		dir_wd, _ := syscall.Open("/", 0x0, 0x0)
		_ = syscall.Chroot("/jail")
		//open dir_wd
		syscall.Fchdir(dir_wd)
		syscall.Close(dir_wd)

		//Break out from jail
		_ = os.Chdir("../../../..")

		//Remount cpu subsystem with following flags:
		//remount,bind,rw - rw is setting as a default flag
		flags := uintptr(syscall.MS_BIND | syscall.MS_REMOUNT)
		err := syscall.Mount(cpu_mp, cpu_mp, "", flags, "")
		if err != nil {
			fmt.Fprint(os.Stderr, err)
		}
		//Open and write a new value to our fake_cg
		file, err := os.OpenFile(cpu_mp+"/fake_cg/cpu.shares", os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprint(os.Stderr, err)
		}
		_, err = file.WriteString("404")
		if err != nil {
			fmt.Fprint(os.Stderr, err)
		}
		file.Close()
	}

	os.Exit(globalFlags.ExitCode)
}
