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

// Package testutil provides shared test utilities for git operations and other common test needs.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type GitRepo struct {
	t   *testing.T
	dir string
}

// Run executes a git command with test-friendly configuration.
// It automatically adds user.name, user.email, gpgsign=false, and protocol.file.allow=always.
func (g *GitRepo) Run(args ...string) string {
	g.t.Helper()
	fullArgs := append(
		[]string{
			"-c", "user.name=Test",
			"-c", "user.email=test@test.com",
			"-c", "commit.gpgsign=false",
			"-c", "tag.gpgsign=false",
			"-c", "protocol.file.allow=always",
		},
		args...,
	)
	cmd := exec.Command("git", fullArgs...)
	cmd.Dir = g.dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		g.t.Fatalf("git %v failed in %s: %v\nOutput: %s", args, g.dir, err, string(output))
	}
	return strings.TrimSpace(string(output))
}

// CommitAll stages all files and creates a commit with the given message.
// Returns the SHA of the created commit.
func (g *GitRepo) CommitAll(message string) string {
	g.t.Helper()
	g.Run("add", ".")
	g.Run("commit", "--allow-empty", "--allow-empty-message", "-m", message)
	return g.RevParse("HEAD")
}

// RevParse returns the SHA for the given ref.
func (g *GitRepo) RevParse(ref string) string {
	g.t.Helper()
	return g.Run("rev-parse", ref)
}

// Dir returns the directory path of the repository.
func (g *GitRepo) Dir() string {
	return g.dir
}

// Tag creates a lightweight tag at the current HEAD.
func (g *GitRepo) Tag(name string) {
	g.t.Helper()
	g.Run("tag", name)
}

// Checkout checks out the given ref.
func (g *GitRepo) Checkout(ref string) {
	g.t.Helper()
	g.Run("checkout", ref)
}

// OriginURL returns the URL of the origin remote.
// Local paths are returned with file:// prefix for compatibility with --depth clones.
func (g *GitRepo) OriginURL() string {
	g.t.Helper()
	url := g.Run("remote", "get-url", "origin")
	if filepath.IsAbs(url) {
		return "file://" + url
	}
	return url
}

// PushAll pushes all branches and tags to origin.
func (g *GitRepo) PushAll() {
	g.t.Helper()
	g.Run("push", "origin", "--all")
	g.Run("push", "origin", "--tags")
}

// Submodule returns a GitRepo for the submodule at the given path.
func (g *GitRepo) Submodule(path string) *GitRepo {
	return &GitRepo{g.t, filepath.Join(g.dir, path)}
}

// AddSubmodule adds a submodule from the given remote URL and returns a GitRepo for it.
func (g *GitRepo) AddSubmodule(remote string, name string) *GitRepo {
	g.t.Helper()
	g.Run("submodule", "add", remote, name)
	return g.Submodule(name)
}

// WriteFile writes content to a file in the repository.
func (g *GitRepo) WriteFile(name string, content string) {
	g.t.Helper()
	err := os.WriteFile(filepath.Join(g.dir, name), []byte(content), 0644)
	if err != nil {
		g.t.Fatalf("Failed to write file %s: %v", name, err)
	}
}

// ReadFile reads a file from the repository.
func (g *GitRepo) ReadFile(name string) string {
	g.t.Helper()
	content, err := os.ReadFile(filepath.Join(g.dir, name))
	if err != nil {
		g.t.Fatalf("Failed to read file %s: %v", name, err)
	}
	return string(content)
}

// GitRepoAt creates a GitRepo for an existing directory (doesn't initialize git).
func GitRepoAt(t *testing.T, dir string) *GitRepo {
	t.Helper()
	return &GitRepo{t: t, dir: dir}
}

// NewGitRepo initializes a new git repository and returns a GitRepo for it.
func NewGitRepo(t *testing.T) *GitRepo {
	t.Helper()
	dir := t.TempDir()
	g := &GitRepo{t: t, dir: dir}
	g.Run("init", "--initial-branch=main")
	return g
}

// NewBareGitRepo initializes a new bare git repository and returns a GitRepo for it.
func NewBareGitRepo(t *testing.T) *GitRepo {
	t.Helper()
	dir := t.TempDir()
	g := &GitRepo{t: t, dir: dir}
	g.Run("init", "--bare", "--initial-branch=main")
	return g
}

// NewClonedGitRepo clones from origin and returns a GitRepo for the clone.
func NewClonedGitRepo(t *testing.T, origin string) *GitRepo {
	t.Helper()
	dir := t.TempDir()
	g := &GitRepo{t: t, dir: dir}
	g.Run("clone", origin, ".")
	return g
}

// Clone clones the repository into a new temp directory and returns a GitRepo for the clone.
// Submodules are recursively initialized.
func (g *GitRepo) Clone() *GitRepo {
	g.t.Helper()
	dir := g.t.TempDir()
	clone := &GitRepo{t: g.t, dir: dir}
	clone.Run("clone", "file://"+g.dir, ".")
	return clone
}

func (g *GitRepo) CloneShallow() *GitRepo {
	g.t.Helper()
	dir := g.t.TempDir()
	clone := &GitRepo{t: g.t, dir: dir}
	clone.Run("clone", "--depth=1", "file://"+g.dir, ".")
	return clone
}

func (g *GitRepo) CloneRecursive() *GitRepo {
	g.t.Helper()
	dir := g.t.TempDir()
	clone := &GitRepo{t: g.t, dir: dir}
	clone.Run("clone", "--recursive", "file://"+g.dir, ".")
	return clone
}

// MakeReadonly makes the repository read-only to prevent accidental modifications.
func (g *GitRepo) MakeReadonly() {
	g.t.Helper()
	err := filepath.Walk(g.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(path, info.Mode()&^0222)
	})
	if err != nil {
		g.t.Fatalf("Failed to make repo readonly: %v", err)
	}
	g.t.Cleanup(func() {
		// Restore write permissions for cleanup
		filepath.Walk(g.dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			os.Chmod(path, info.Mode()|0200)
			return nil
		})
	})
}

// SampleRepo creates a bare repo with two commits tagged v1 and v2.
// v1 contains file.txt with "content-v1", v2 contains "content-v2".
// The returned repo is read-only; use Clone() to get a working copy.
func SampleRepo(t *testing.T) *GitRepo {
	t.Helper()
	origin := NewBareGitRepo(t)
	work := NewClonedGitRepo(t, origin.Dir())
	work.WriteFile("file.txt", "content-v1")
	work.CommitAll("v1")
	work.Tag("v1")
	work.WriteFile("file.txt", "content-v2")
	work.CommitAll("v2")
	work.Tag("v2")
	work.PushAll()
	origin.MakeReadonly()
	return origin
}

// SampleRepoWithSubmodule creates a bare repo with a submodule "submodule".
// The main repo has tags v1 and v2, where v1 has submodule at v1 and v2 has submodule at v2.
// The returned repo is read-only; use Clone() to get a working copy.
func SampleRepoWithSubmodule(t *testing.T) *GitRepo {
	t.Helper()
	subOrigin := SampleRepo(t)
	mainOrigin := NewBareGitRepo(t)
	mainWork := mainOrigin.Clone()
	mainWork.WriteFile("main.txt", "main-content")
	mainWork.CommitAll("initial")
	sub := mainWork.AddSubmodule(subOrigin.Dir(), "submodule")
	sub.Checkout("v1")
	mainWork.CommitAll("v1")
	mainWork.Tag("v1")
	sub.Checkout("v2")
	mainWork.CommitAll("v2")
	mainWork.Tag("v2")
	mainWork.PushAll()
	mainOrigin.MakeReadonly()
	return mainOrigin
}
