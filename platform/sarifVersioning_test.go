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
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"os"
	"testing"
)

func TestGetBranchName(t *testing.T) {
	dir, err := os.MkdirTemp("", "test-repository")
	if err != nil {
		t.Fatal(err)
	}
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(dir)

	_, _, ret, err := utils.RunCmdRedirectOutput(dir, "git", "init", "--initial-branch=my-branch")
	if err != nil {
		t.Fatal(err)
	}
	if ret != 0 {
		t.Fatalf("git init failed with exit code %d", ret)
	}

	branch, err := getBranchName(dir)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "my-branch" {
		t.Fatalf("Incorrect branch name: '%s' (expected 'my-branch')", branch)
	}

	_, _, ret, err = utils.RunCmdRedirectOutput(dir, "git", "commit", "--allow-empty", "-m", "commit")
	if err != nil {
		t.Fatal(err)
	}
	if ret != 0 {
		t.Fatalf("git commit failed with exit code %d", ret)
	}

	branch, err = getBranchName(dir)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "my-branch" {
		t.Fatalf("Incorrect branch name: '%s' (expected 'my-branch')", branch)
	}

	_, _, ret, err = utils.RunCmdRedirectOutput(dir, "git", "switch", "--detach")
	if err != nil {
		t.Fatal(err)
	}
	if ret != 0 {
		t.Fatalf("git switch failed with exit code %d", ret)
	}

	branch, err = getBranchName(dir)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "" {
		t.Fatalf("Incorrect branch name: '%s' (expected '<empty>')", branch)
	}
}
