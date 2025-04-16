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
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
