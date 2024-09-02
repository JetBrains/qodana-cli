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
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestGitDiffNamesOnly(t *testing.T) {
	temp, _ := os.MkdirTemp("", "")
	projectPath := createNativeProject(t, "casamples")
	defer deferredCleanup(projectPath)

	strings := [2]string{"", "duplicates"}
	oldRev := "12bf1bf4dd267b972e05a586cd7dfd3ad9a5b4a0"
	newRev := "d6f64adb1d594ea4bb6e40d027e1e863726c4323"

	// Iterate over the array using a for loop.
	for _, str := range strings {
		path := filepath.Join(projectPath, str)
		diff, err := GitDiffNameOnly(path, oldRev, newRev, temp)
		if err != nil {
			t.Fatalf("GitDiffNameOnly() error = %v", err)
		}
		diffLegacy := GitDiffNameOnlyLegacy(path, oldRev, newRev)
		sort.Strings(diff)
		sort.Strings(diffLegacy)
		equal := reflect.DeepEqual(diff, diffLegacy)
		if !equal {
			t.Fatalf("Old and new diffs are not equal: old: %v new: %v", diffLegacy, diff)
		}
		branch, _ := GitBranch(path, temp)
		branchLegacy := GitBranchLegacy(path)
		if branch != branchLegacy {
			t.Fatalf("Old and new branch are not equal: old: %v new: %v", branchLegacy, branch)
		}
		revision, _ := GitCurrentRevision(path, temp)
		revisionLegacy := GitCurrentRevisionLegacy(path)
		if revision != revisionLegacy {
			t.Fatalf("Old and new revision are not equal: old: %v new: %v", revisionLegacy, revision)
		}
		remoteUrl, _ := GitRemoteUrl(path, temp)
		remoteUrlLegacy := GitRemoteUrlLegacy(path)
		if remoteUrl != remoteUrlLegacy {
			t.Fatalf("Old and new url are not equal: old: %v new: %v", remoteUrlLegacy, remoteUrl)
		}
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
	err = gitClone("https://github.com/JetBrains/code-analytics-examples", location)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func gitClone(repoURL, directory string) error {
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
	return nil
}
