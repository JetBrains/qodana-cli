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
	repo := NewGitRepo(t)
	repo.CommitAll("commit")
	logdir := t.TempDir()

	repo.Run("checkout", "-b", "test-branch")
	repo.CommitAll("branch-commit")

	err := CheckoutAndUpdateSubmodule(repo.Dir(), "main", false, logdir)
	assert.NoError(t, err)

	err = CheckoutAndUpdateSubmodule(repo.Dir(), "test-branch", true, logdir)
	assert.NoError(t, err)
}

func TestCheckoutAndUpdateSubmodule_SubmoduleNotCheckedOut(t *testing.T) {
	GitAllowFileProtocol(t)
	logdir := t.TempDir()
	repo := SampleRepoWithSubmodule(t).Clone()

	// Submodule is not initialized after non-recursive clone.
	// CheckoutAndUpdateSubmodule should initialize and checkout it.
	err := CheckoutAndUpdateSubmodule(repo.Dir(), "v1", true, logdir)
	assert.NoError(t, err)
	assert.Equal(t, "content-v1", repo.Submodule("submodule").ReadFile("file.txt"))
}

func TestCheckoutAndUpdateSubmodule_ShallowRepoAndSubmoduleNotCheckedOut(t *testing.T) {
	GitAllowFileProtocol(t)
	logdir := t.TempDir()
	repo := SampleRepoWithSubmodule(t).CloneShallow()

	err := CheckoutAndUpdateSubmodule(repo.Dir(), "v1", true, logdir)
	assert.Error(t, err)
	println(err.Error())
	assert.Contains(t, err.Error(), "shallow")
}

func TestCheckoutAndUpdateSubmodule_ShallowSubmodule(t *testing.T) {
	GitAllowFileProtocol(t)
	logdir := t.TempDir()
	repo := SampleRepoWithSubmodule(t).CloneShallow()
	submodule := repo.Submodule("submodule")

	// Get submodule remote URL before removing it
	submoduleOrigin := submodule.OriginURL()

	// Replace submodule with shallow clone
	err := os.RemoveAll(submodule.Dir())
	assert.NoError(t, err)
	repo.Run("clone", "--depth=1", submoduleOrigin, "submodule")

	err = CheckoutAndUpdateSubmodule(repo.Dir(), "v1", true, logdir)
	assert.Error(t, err)
	println(err.Error())
}

// TestCheckoutAndUpdateSubmodule_OrphanedGitmodulesEntry tests the case where
// .gitmodules declares a submodule that doesn't exist in git cache.
// This can happen when a user manually edits .gitmodules without running git submodule add.
func TestCheckoutAndUpdateSubmodule_OrphanedGitmodulesEntry(t *testing.T) {
	GitAllowFileProtocol(t)
	logdir := t.TempDir()
	repo := SampleRepoWithSubmodule(t).CloneRecursive()

	// Add a fake submodule entry to .gitmodules without actually adding it to git cache
	gitmodules := repo.ReadFile(".gitmodules")
	gitmodules += `
[submodule "fake-submodule"]
	path = fake/path
	url = https://example.com/fake.git
`
	repo.WriteFile(".gitmodules", gitmodules)
	repo.CommitAll("add orphaned gitmodules entry")

	// CheckoutAndUpdateSubmodule should succeed - it should ignore the orphaned entry
	err := CheckoutAndUpdateSubmodule(repo.Dir(), "HEAD", true, logdir)
	assert.NoError(t, err)
}

// TestCheckoutAndUpdateSubmodule_OrphanedCacheEntry tests the case where
// git cache has a submodule that's not declared in .gitmodules.
// This can happen when a user removes the .gitmodules entry but doesn't properly remove the submodule.
func TestCheckoutAndUpdateSubmodule_OrphanedCacheEntry(t *testing.T) {
	GitAllowFileProtocol(t)
	logdir := t.TempDir()
	repo := SampleRepoWithSubmodule(t).CloneRecursive()

	// Remove the submodule entry from .gitmodules but keep it in git cache
	repo.Run("config", "--file", ".gitmodules", "--remove-section", "submodule.submodule")
	repo.CommitAll("remove gitmodules entry but keep cache")

	// CheckoutAndUpdateSubmodule should succeed - it should ignore the orphaned cache entry
	err := CheckoutAndUpdateSubmodule(repo.Dir(), "HEAD", true, logdir)
	assert.NoError(t, err)
}

// TestCheckoutAndUpdateSubmodule_SubmoduleNameDiffersFromPath tests the case where
// a submodule has a name that differs from its path (e.g., nested paths like libs/utils/helper).
func TestCheckoutAndUpdateSubmodule_SubmoduleNameDiffersFromPath(t *testing.T) {
	GitAllowFileProtocol(t)
	logdir := t.TempDir()

	// Create a submodule origin
	subOrigin := SampleRepo(t)

	// Create main repo with a submodule where name differs from path
	mainOrigin := NewBareGitRepo(t)
	mainWork := mainOrigin.Clone()
	mainWork.WriteFile("main.txt", "content")
	mainWork.CommitAll("initial")

	// Add submodule with name "my-lib" at path "libs/utils/helper"
	mainWork.AddSubmoduleWithName(subOrigin.Dir(), "libs/utils/helper", "my-lib")
	mainWork.Submodule("libs/utils/helper").Checkout("v1")
	mainWork.CommitAll("v1")
	mainWork.Tag("v1")

	mainWork.Submodule("libs/utils/helper").Checkout("v2")
	mainWork.CommitAll("v2")
	mainWork.Tag("v2")
	mainWork.PushAll()

	// Clone and test checkout
	repo := mainOrigin.CloneRecursive()

	// Checkout v1 - submodule should be updated to v1
	err := CheckoutAndUpdateSubmodule(repo.Dir(), "v1", true, logdir)
	assert.NoError(t, err)
	assert.Equal(t, "content-v1", repo.Submodule("libs/utils/helper").ReadFile("file.txt"))

	// Checkout v2 - submodule should be updated to v2
	err = CheckoutAndUpdateSubmodule(repo.Dir(), "v2", true, logdir)
	assert.NoError(t, err)
	assert.Equal(t, "content-v2", repo.Submodule("libs/utils/helper").ReadFile("file.txt"))
}

func TestGetDeclaredSubmodules_NoEmptyStrings(t *testing.T) {
	logdir := t.TempDir()
	repo := SampleRepoWithSubmodule(t).CloneRecursive()

	submodules, err := getDeclaredSubmodules(repo.Dir(), logdir)
	assert.NoError(t, err)
	assert.NotEmpty(t, submodules)

	for i, path := range submodules {
		assert.NotEmpty(t, path, "getDeclaredSubmodules returned empty string at index %d", i)
	}
}

func TestGetSubmodules_NoEmptyStrings(t *testing.T) {
	logdir := t.TempDir()
	repo := SampleRepoWithSubmodule(t).CloneRecursive()

	submodules, err := getSubmodules(repo.Dir(), logdir)
	assert.NoError(t, err)
	assert.NotEmpty(t, submodules)

	for i, path := range submodules {
		assert.NotEmpty(t, path, "getSubmodules returned empty string at index %d", i)
	}
}
