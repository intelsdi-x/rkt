// Copyright 2016 The rkt Authors
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
	"os"
	"strings"
	"time"
)

func main() {
	appendLine("TIME " + time.Now().Format(time.RFC3339Nano) + "\n")
	hostname := getHostname()
	appendLine("ID " + hostname + "\n")
	for {
		var x uint64
		for i := 0; i < 1000000000; i++ {
			x += uint64(i)
		}
	}
}

func appendLine(line string) {
	f, err := os.OpenFile("/tmp/benchmarking_info", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	if _, err = f.WriteString(line); err != nil {
		panic(err)
	}
}

func getHostname() string {
	file, err := os.Open("/etc/hostname")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "rkt-") {
			sl := strings.SplitAfter(scanner.Text(), "rkt-")
			return sl[len(sl)-1]
		}
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}
	return "ERROR"
}
