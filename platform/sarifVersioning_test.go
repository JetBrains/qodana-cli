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

	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
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

	runCommand(t, dir, "git", "init", "--initial-branch=my-branch")
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

	runCommand(t, dir, "git", "switch", "--detach")
	branch, err = getBranchName(dir)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "" {
		t.Fatalf("Incorrect branch name: '%s' (expected <empty>)", branch)
	}
}

func TestGetVersionDetailsBranchFromEnvironment(t *testing.T) {
	dir, err := os.MkdirTemp("", "repo-TestGetVersionDetailsBranch")
	if err != nil {
		t.Fatal(err)
	}
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(dir)

	assertBranchName := func(expected string) {
		version_info, err := GetVersionDetails(dir)
		if err != nil {
			t.Fatal(err)
		}
		if version_info.Branch != expected {
			t.Fatalf("Incorrect branch name: '%s' (expected '%s')", version_info.Branch, expected)
		}
	}

	runCommand(t, dir, "git", "init", "--initial-branch=my-branch")
	runGitCommit(t, dir)
	runCommand(t, dir, "git", "switch", "--detach")

	os.Setenv("QODANA_BRANCH", "QODANA_BRANCH")
	os.Setenv("CI", "true")
	os.Setenv("CI_COMMIT_BRANCH", "CI_COMMIT_BRANCH")
	assertBranchName("QODANA_BRANCH")

	os.Setenv("QODANA_BRANCH", "QODANA_BRANCH")
	os.Unsetenv("CI")
	os.Unsetenv("CI_COMMIT_BRANCH")
	assertBranchName("QODANA_BRANCH")

	os.Unsetenv("QODANA_BRANCH")
	os.Setenv("CI", "true")
	os.Setenv("CI_COMMIT_BRANCH", "CI_COMMIT_BRANCH")
	assertBranchName("CI_COMMIT_BRANCH")

	os.Unsetenv("QODANA_BRANCH")
	os.Setenv("CI", "false")
	os.Setenv("CI_COMMIT_BRANCH", "CI_COMMIT_BRANCH")
	assertBranchName("")

	os.Unsetenv("QODANA_BRANCH")
	os.Unsetenv("CI")
	os.Setenv("CI_COMMIT_BRANCH", "CI_COMMIT_BRANCH")
	assertBranchName("")
}

func runCommand(t *testing.T, cwd string, args ...string) (string, string) {
	stdout, stderr, ret, err := utils.RunCmdRedirectOutput(cwd, args...)
	if err != nil {
		t.Fatal(err)
	}
	if ret != 0 {
		t.Fatalf("%q failed with exit code %d.\nStdout was: %q\nStderr was: %q", args, ret, stdout, stderr)
	}

	return stdout, stderr
}

func runGitCommit(t *testing.T, cwd string) {
	runCommand(t, cwd,
		"git", "-c", "user.name=platform/sarifVersioning_test.go", "-c", "user.email=<>",
		"commit", "--allow-empty", "-m", "commit",
	)
}
