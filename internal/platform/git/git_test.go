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

package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

const (
	REV       = "aa1fe0eac28bbc363036b39ab937b081f06f407a"
	MALFORMED = "aabbb0eac28bbc363036b39ab937b081f06f407a"
	BRANCH    = "my-branch"
	REPO      = "https://github.com/JetBrains/code-analytics-examples"
)

func TestGitFunctionalityChange(t *testing.T) {
	temp, _ := os.MkdirTemp("", "")
	projectPath := createNativeProject(t, "casamples")
	defer deferredCleanup(projectPath)

	branch, _ := Branch(projectPath, temp)
	branchLegacy := BranchLegacy(projectPath)
	if branch != branchLegacy {
		t.Fatalf("Old and new branch are not equal: old: %v new: %v", branchLegacy, branch)
	}
	if branch != BRANCH {
		t.Fatalf("New and expected branch are not equal: new: %v expected: %v", branch, BRANCH)
	}
	revision, _ := CurrentRevision(projectPath, temp)
	revisionLegacy := CurrentRevisionLegacy(projectPath)
	if revision != revisionLegacy {
		t.Fatalf("Old and new revision are not equal: old: %v new: %v", revisionLegacy, revision)
	}
	if revision != REV {
		t.Fatalf("New and expected revision are not equal: new: %v expected: %v", revision, REV)
	}
	remoteUrl, _ := RemoteUrl(projectPath, temp)
	remoteUrlLegacy := RemoteUrlLegacy(projectPath)
	if remoteUrl != remoteUrlLegacy {
		t.Fatalf("Old and new url are not equal: old: %v new: %v", remoteUrlLegacy, remoteUrl)
	}
	if remoteUrl != REPO {
		t.Fatalf("New and expected repo urls are not equal: new: %v expected: %v", remoteUrl, REPO)
	}
	rootPath, _ := Root(projectPath, temp)
	if filepath.ToSlash(rootPath) != filepath.ToSlash(projectPath) {
		t.Fatalf("Computed git root path are not equal: new: %v expected: %v", rootPath, projectPath)
	}
	existsCorrect := RevisionExists(projectPath, REV, temp)
	if existsCorrect != true {
		t.Fatalf("Revision %v is not found in project %v", REV, projectPath)
	}
	dontExists := RevisionExists(projectPath, MALFORMED, temp)
	if dontExists {
		t.Fatalf("Revision %v is found in project %v", MALFORMED, projectPath)
	}
}

func TestGitRunReportsErrors(t *testing.T) {
	tempDir := t.TempDir()
	_, _, err := gitRun(tempDir, []string{"bad-command"}, tempDir)
	assert.Error(t, err)
}

func TestRevParse(t *testing.T) {
	reSha1 := regexp.MustCompile("^[0-9a-f]{40}$")
	logdir := t.TempDir()

	repo := NewGitRepo(t)
	repo.CommitAll("commit")

	// Resolving head
	headSha, err := RevParse(repo.Dir(), "HEAD", logdir)
	assert.NoError(t, err)
	assert.Regexp(t, reSha1, headSha)

	// Resolving branch
	branchSha1, err := RevParse(repo.Dir(), "main", logdir)
	assert.NoError(t, err)
	assert.Equal(t, headSha, branchSha1)

	branchSha2, err := RevParse(repo.Dir(), "refs/heads/main", logdir)
	assert.NoError(t, err)
	assert.Equal(t, headSha, branchSha2)

	// Resolving tag
	repo.Tag("v1.0.0")
	tagSha1, err := RevParse(repo.Dir(), "v1.0.0", logdir)
	assert.NoError(t, err)
	assert.Equal(t, headSha, tagSha1)

	tagSha2, err := RevParse(repo.Dir(), "refs/tags/v1.0.0", logdir)
	assert.NoError(t, err)
	assert.Equal(t, headSha, tagSha2)

	// Resolving short SHA1
	shortSha, err := RevParse(repo.Dir(), headSha[:5], logdir)
	assert.NoError(t, err)
	assert.Equal(t, headSha, shortSha)

	// Resolving full SHA1
	headShaSha, err := RevParse(repo.Dir(), headSha, logdir)
	assert.NoError(t, err)
	assert.Equal(t, headSha, headShaSha)
}

func deferredCleanup(path string) {
	err := os.RemoveAll(path)
	if err != nil {
		log.Fatal(err)
	}
}

func createNativeProject(t *testing.T, name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	location := filepath.Join(home, ".qodana_scan_", name)
	err = gitClone("https://github.com/JetBrains/code-analytics-examples", location, REV, BRANCH)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func gitClone(repoURL, directory string, revision string, branch string) error {
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		err = os.RemoveAll(directory)
		if err != nil {
			return err
		}
	}
	cmd := exec.Command("git", "clone", repoURL, directory)
	err := cmd.Run()
	if err != nil {
		return err
	}
	cmd = exec.Command("git", "checkout", revision)
	cmd.Dir = directory
	err = cmd.Run()
	if err != nil {
		return err
	}
	cmd = exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = directory
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func TestReset(t *testing.T) {
	repo := NewGitRepo(t)
	repo.CommitAll("commit")
	logdir := t.TempDir()

	headSha, err := RevParse(repo.Dir(), "HEAD", logdir)
	assert.NoError(t, err)

	repo.CommitAll("commit")

	err = Reset(repo.Dir(), headSha, logdir)
	assert.NoError(t, err)
}

func TestCheckout(t *testing.T) {
	repo := NewGitRepo(t)
	repo.CommitAll("commit")
	logdir := t.TempDir()

	repo.Run("checkout", "-b", "test-branch")
	repo.CommitAll("commit")

	err := checkout(repo.Dir(), "main", false, logdir)
	assert.NoError(t, err)

	err = checkout(repo.Dir(), "test-branch", true, logdir)
	assert.NoError(t, err)
}

func TestClean(t *testing.T) {
	repo := NewGitRepo(t)
	repo.CommitAll("commit")
	logdir := t.TempDir()

	repo.WriteFile("untracked.txt", "test")

	err := Clean(repo.Dir(), logdir)
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(repo.Dir(), "untracked.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestRevisions(t *testing.T) {
	repo := NewGitRepo(t)
	repo.CommitAll("commit")
	repo.CommitAll("commit")
	repo.CommitAll("commit")

	revisions := Revisions(repo.Dir())
	assert.GreaterOrEqual(t, len(revisions), 3)
}

func TestRemoteUrl(t *testing.T) {
	logdir := t.TempDir()
	repo := NewGitRepo(t)
	repo.CommitAll("commit")

	repo.Run("remote", "add", "origin", "https://github.com/test/repo.git")

	url, err := RemoteUrl(repo.Dir(), logdir)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/test/repo.git", url)
}

func TestBranch(t *testing.T) {
	logdir := t.TempDir()
	repo := NewGitRepo(t)
	repo.CommitAll("commit")

	branch, err := Branch(repo.Dir(), logdir)
	assert.NoError(t, err)
	assert.Equal(t, "main", branch)
}

func TestCurrentRevision(t *testing.T) {
	logdir := t.TempDir()
	repo := NewGitRepo(t)
	repo.CommitAll("commit")

	rev, err := CurrentRevision(repo.Dir(), logdir)
	assert.NoError(t, err)
	assert.Len(t, rev, 40)
}

func TestSubmoduleUpdate(t *testing.T) {
	repo := NewGitRepo(t)
	repo.CommitAll("commit")
	logdir := t.TempDir()

	err := submoduleUpdate(repo.Dir(), false, logdir)
	assert.NoError(t, err)

	err = submoduleUpdate(repo.Dir(), true, logdir)
	assert.NoError(t, err)
}

func TestResetBack(t *testing.T) {
	repo := NewGitRepo(t)
	repo.CommitAll("commit")
	logdir := t.TempDir()

	headSha, _ := RevParse(repo.Dir(), "HEAD", logdir)
	repo.CommitAll("second-commit")
	err := Reset(repo.Dir(), headSha, logdir)
	if err != nil {
		t.Fatal(err)
	}

	err = ResetBack(repo.Dir(), logdir)
	assert.NoError(t, err)
}

func TestCheckoutAndUpdateSubmodule(t *testing.T) {
	GitAllowFileProtocol(t)

	t.Run("BasicCheckout", func(t *testing.T) {
		repo := NewGitRepo(t)
		repo.CommitAll("commit")
		logdir := t.TempDir()

		repo.Run("checkout", "-b", "test-branch")
		repo.CommitAll("branch-commit")

		err := CheckoutAndUpdateSubmodule(repo.Dir(), "main", false, logdir)
		assert.NoError(t, err)

		err = CheckoutAndUpdateSubmodule(repo.Dir(), "test-branch", true, logdir)
		assert.NoError(t, err)
	})

	t.Run("SubmoduleNotCheckedOut", func(t *testing.T) {
		logdir := t.TempDir()
		repo := SampleRepoWithSubmodule(t).Clone()

		// Submodule is not initialized after non-recursive clone.
		// CheckoutAndUpdateSubmodule should initialize and checkout it.
		err := CheckoutAndUpdateSubmodule(repo.Dir(), "v1", true, logdir)
		assert.NoError(t, err)
		assert.Equal(t, "content-v1", repo.Submodule("submodules/regular").ReadFile("file.txt"))
	})

	t.Run("ShallowRepoAndSubmoduleNotCheckedOut", func(t *testing.T) {
		logdir := t.TempDir()
		repo := SampleRepoWithSubmodule(t).CloneShallow()

		err := CheckoutAndUpdateSubmodule(repo.Dir(), "v1", true, logdir)
		assert.Error(t, err)
		println(err.Error())
		assert.Contains(t, err.Error(), "shallow")
	})

	t.Run("ShallowSubmodule", func(t *testing.T) {
		logdir := t.TempDir()
		repo := SampleRepoWithSubmodule(t).CloneShallow()
		submodule := repo.Submodule("submodules/regular")

		// Get submodule remote URL before removing it
		submoduleOrigin := submodule.OriginURL()

		// Replace submodule with shallow clone
		err := os.RemoveAll(submodule.Dir())
		assert.NoError(t, err)
		repo.Run("clone", "--depth=1", submoduleOrigin, "submodules/regular")

		err = CheckoutAndUpdateSubmodule(repo.Dir(), "v1", true, logdir)
		assert.Error(t, err)
		println(err.Error())
	})

	t.Run("FakeSubmodule", func(t *testing.T) {
		logdir := t.TempDir()
		repo := SampleRepoWithFakeSubmodule(t).CloneRecursive()

		err := CheckoutAndUpdateSubmodule(repo.Dir(), "HEAD", true, logdir)
		assert.NoError(t, err)
	})

	t.Run("OrphanedSubmodule", func(t *testing.T) {
		logdir := t.TempDir()
		repo := SampleRepoWithOrphanedSubmodule(t).CloneRecursive()

		// CheckoutAndUpdateSubmodule should succeed - it should ignore the orphaned cache entry
		err := CheckoutAndUpdateSubmodule(repo.Dir(), "HEAD", true, logdir)
		assert.NoError(t, err)
	})
}

func TestGetSubmodules(t *testing.T) {
	GitAllowFileProtocol(t)
	logdir := t.TempDir()

	t.Run("Basic", func(t *testing.T) {
		repo := SampleRepoWithSubmodule(t).CloneRecursive()

		declared, err := getDeclaredSubmodules(repo.Dir(), logdir)
		assert.NoError(t, err)
		assert.Equal(t, []string{"submodules/regular"}, declared)

		submodules, err := getSubmodules(repo.Dir(), logdir)
		assert.NoError(t, err)
		assert.Equal(t, []string{"submodules/regular"}, submodules)
	})

	t.Run("FakeSubmodule", func(t *testing.T) {
		repo := SampleRepoWithFakeSubmodule(t).CloneRecursive()

		// .gitmodules declares both "submodules/regular" and "submodules/fake"
		declared, err := getDeclaredSubmodules(repo.Dir(), logdir)
		assert.NoError(t, err)
		assert.Equal(t, []string{"submodules/regular", "submodules/fake"}, declared)

		// Only "submodules/regular" exists in git cache
		submodules, err := getSubmodules(repo.Dir(), logdir)
		assert.NoError(t, err)
		assert.Equal(t, []string{"submodules/regular"}, submodules)
	})

	t.Run("OrphanedSubmodule", func(t *testing.T) {
		repo := SampleRepoWithOrphanedSubmodule(t).CloneRecursive()

		// .gitmodules only has "submodules/regular" (orphaned entry was removed)
		declared, err := getDeclaredSubmodules(repo.Dir(), logdir)
		assert.NoError(t, err)
		assert.Equal(t, []string{"submodules/regular"}, declared)

		// getSubmodules returns only declared submodules that exist in cache
		submodules, err := getSubmodules(repo.Dir(), logdir)
		assert.NoError(t, err)
		assert.Equal(t, []string{"submodules/regular"}, submodules)
	})
}
