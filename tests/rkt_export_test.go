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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/coreos/rkt/common"
	"github.com/coreos/rkt/tests/testutils"
)

func TestExport(t *testing.T) {
	const (
		testFile    = "test.txt"
		testContent = "ThisIsATest"
	)

	ctx := testutils.NewRktRunCtx()
	defer ctx.Cleanup()

	type testCfg struct {
		runArgs        string
		writeArgs      string
		readArgs       string
		expectedResult string
		unmountOverlay bool
	}

	tests := []testCfg{
		{
			"--no-overlay --insecure-options=image",
			"--write-file --file-name=" + testFile + " --content=" + testContent,
			"--read-file --file-name=" + testFile,
			testContent,
			false,
		},
	}

	// Need to do both checks
	if common.SupportsUserNS() && checkUserNS() == nil {
		tests = append(tests, []testCfg{
			{
				"--private-users --no-overlay --insecure-options=image",
				"--write-file --file-name=" + testFile + " --content=" + testContent,
				"--read-file --file-name=" + testFile,
				testContent,
				false,
			},
		}...)
	}

	if common.SupportsOverlay() {
		tests = append(tests, []testCfg{
			{
				"--insecure-options=image",
				"--write-file --file-name=" + testFile + " --content=" + testContent,
				"--read-file --file-name=" + testFile,
				testContent,
				false,
			},
			// Test unmounting overlay to simulate a reboot
			{
				"--insecure-options=image",
				"--write-file --file-name=" + testFile + " --content=" + testContent,
				"--read-file --file-name=" + testFile,
				testContent,
				true,
			},
		}...)
	}

	for _, tt := range tests {
		tmpDir := createTempDirOrPanic("rkt-TestExport-tmp-")
		defer os.RemoveAll(tmpDir)

		tmpTestAci := filepath.Join(tmpDir, "test.aci")

		// Prepare the image with modifications
		const runInspect = "%s %s %s %s --exec=/inspect -- %s"
		prepareCmd := fmt.Sprintf(runInspect, ctx.Cmd(), "prepare", tt.runArgs, getInspectImagePath(), tt.writeArgs)
		t.Logf("Preparing 'inspect --write-file'")
		uuid := runRktAndGetUUID(t, prepareCmd)

		runCmd := fmt.Sprintf("%s run-prepared %s", ctx.Cmd(), uuid)
		t.Logf("Running 'inspect --write-file'")
		child := spawnOrFail(t, runCmd)
		waitOrFail(t, child, 0)

		if tt.unmountOverlay {
			unmountPod(t, ctx, uuid, true)
		}

		// Export the image
		exportCmd := fmt.Sprintf("%s export %s %s", ctx.Cmd(), uuid, tmpTestAci)
		t.Logf("Running 'export'")
		child = spawnOrFail(t, exportCmd)
		waitOrFail(t, child, 0)

		// Run the newly created ACI and check the output
		readCmd := fmt.Sprintf(runInspect, ctx.Cmd(), "run", tt.runArgs, tmpTestAci, tt.readArgs)
		t.Logf("Running 'inspect --read-file'")
		child = spawnOrFail(t, readCmd)
		if tt.expectedResult != "" {
			if _, out, err := expectRegexWithOutput(child, tt.expectedResult); err != nil {
				t.Fatalf("expected %q but not found: %v\n%s", tt.expectedResult, err, out)
			}
		}
		waitOrFail(t, child, 0)

		// run garbage collector on pods and images
		runGC(t, ctx)
		runImageGC(t, ctx)
	}
}
