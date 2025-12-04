/*
 * Copyright 2021-2024 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package platform

import (
	"os"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/utils"
)

func TestGetBranchName(t *testing.T) {
	dir, err := os.MkdirTemp("", "repo-TestGetBranchName")
	if err != nil {
		t.Fatal(err)
	}
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(dir)

	runCommand(t, dir, "git init --initial-branch=my-branch")
	branch, err := getBranchName(dir)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "my-branch" {
		t.Fatalf("Incorrect branch name: '%s' (expected 'my-branch')", branch)
	}

	runGitCommit(t, dir)
	branch, err = getBranchName(dir)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "my-branch" {
		t.Fatalf("Incorrect branch name: '%s' (expected 'my-branch')", branch)
	}

	runCommand(t, dir, "git switch --detach")
	branch, err = getBranchName(dir)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "" {
		t.Fatalf("Incorrect branch name: '%s' (expected <empty>)", branch)
	}
}

func runCommand(t *testing.T, cwd string, command string) (string, string) {
	stdout, stderr, ret, err := utils.RunCmdRedirectOutput(cwd, command)
	if err != nil {
		t.Fatal(err)
	}
	if ret != 0 {
		t.Fatalf("%q failed with exit code %d.\nStdout was: %q\nStderr was: %q", command, ret, stdout, stderr)
	}

	return stdout, stderr
}

func runGitCommit(t *testing.T, cwd string) {
	runCommand(t, cwd,
		"git -c user.name=platform/sarifVersioning_test.go -c user.email=none commit --allow-empty -m commit",
	)
}
