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
		"git -c user.name=platform/sarifVersioning_test.go -c user.email=none -c commit.gpgsign=false commit --allow-empty -m commit",
	)
}

func TestGetRevisionId(t *testing.T) {
	dir, err := os.MkdirTemp("", "repo-TestGetRevisionId")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	runCommand(t, dir, "git init --initial-branch=main")
	runGitCommit(t, dir)

	rev, err := getRevisionId(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rev) != 40 {
		t.Fatalf("Expected 40 char SHA, got: %s", rev)
	}
}

func TestGetRevisionId_NoRepo(t *testing.T) {
	dir, err := os.MkdirTemp("", "no-repo-TestGetRevisionId")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	_, err = getRevisionId(dir)
	if err == nil {
		t.Fatal("Expected error for non-git directory")
	}
}

func TestGetRepositoryUri(t *testing.T) {
	dir, err := os.MkdirTemp("", "repo-TestGetRepositoryUri")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	runCommand(t, dir, "git init --initial-branch=main")
	runGitCommit(t, dir)

	// Without remote, should return file:// URI
	uri, err := getRepositoryUri(dir)
	if err != nil {
		t.Fatal(err)
	}
	if uri == "" {
		t.Fatal("Expected non-empty URI")
	}
}

func TestGetLastAuthorName(t *testing.T) {
	dir, err := os.MkdirTemp("", "repo-TestGetLastAuthorName")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	runCommand(t, dir, "git init --initial-branch=main")
	runGitCommit(t, dir)

	name := getLastAuthorName(dir)
	if name == "" {
		t.Fatal("Expected non-empty author name")
	}
}

func TestGetLastAuthorName_NoRepo(t *testing.T) {
	dir, err := os.MkdirTemp("", "no-repo-TestGetLastAuthorName")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	name := getLastAuthorName(dir)
	if name != "" {
		t.Fatalf("Expected empty name for non-repo, got: %s", name)
	}
}

func TestGetAuthorEmail(t *testing.T) {
	dir, err := os.MkdirTemp("", "repo-TestGetAuthorEmail")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	runCommand(t, dir, "git init --initial-branch=main")
	runGitCommit(t, dir)

	email := getAuthorEmail(dir)
	// The email is set in runGitCommit to "none"
	if email == "" {
		t.Fatal("Expected non-empty author email")
	}
}

func TestGetAuthorEmail_NoRepo(t *testing.T) {
	dir, err := os.MkdirTemp("", "no-repo-TestGetAuthorEmail")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	email := getAuthorEmail(dir)
	if email != "" {
		t.Fatalf("Expected empty email for non-repo, got: %s", email)
	}
}

func TestGetVersionDetails(t *testing.T) {
	dir, err := os.MkdirTemp("", "repo-TestGetVersionDetails")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	runCommand(t, dir, "git init --initial-branch=main")
	runGitCommit(t, dir)

	details, err := GetVersionDetails(dir)
	if err != nil {
		t.Fatal(err)
	}

	if details.Branch != "main" {
		t.Errorf("Expected branch 'main', got '%s'", details.Branch)
	}
	if details.RevisionId == "" {
		t.Error("Expected non-empty revision ID")
	}
	if details.RepositoryUri == "" {
		t.Error("Expected non-empty repository URI")
	}
	if details.Properties == nil {
		t.Error("Expected non-nil properties")
	}
}

func TestGetVersionDetails_WithEnvOverrides(t *testing.T) {
	dir, err := os.MkdirTemp("", "repo-TestGetVersionDetailsEnv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	runCommand(t, dir, "git init --initial-branch=main")
	runGitCommit(t, dir)

	// Set environment overrides
	_ = os.Setenv("QODANA_REMOTE_URL", "https://github.com/test/repo")
	_ = os.Setenv("QODANA_BRANCH", "feature-branch")
	_ = os.Setenv("QODANA_REVISION", "abc123def456")
	defer func() {
		_ = os.Unsetenv("QODANA_REMOTE_URL")
		_ = os.Unsetenv("QODANA_BRANCH")
		_ = os.Unsetenv("QODANA_REVISION")
	}()

	details, err := GetVersionDetails(dir)
	if err != nil {
		t.Fatal(err)
	}

	if details.RepositoryUri != "https://github.com/test/repo" {
		t.Errorf("Expected overridden remote URL, got '%s'", details.RepositoryUri)
	}
	if details.Branch != "feature-branch" {
		t.Errorf("Expected overridden branch, got '%s'", details.Branch)
	}
	if details.RevisionId != "abc123def456" {
		t.Errorf("Expected overridden revision, got '%s'", details.RevisionId)
	}
}
