/*
 * Copyright 2021-2026 JetBrains s.r.o.
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

package edict

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	distilleryTestRepositoryURL   = "ssh://git@git.jetbrains.team/sa/distillery-test.git"
	distilleryTestFixtureRevision = "a7fa96f48394b6e1e0d2d46e45403f0b3e3e18c3"
)

type distilleryTestCheckout struct {
	TestRoot            string
	RepositoryDirectory string
	GitBinary           string
}

func cloneDistilleryTestRepository(t *testing.T, expectedHead string) distilleryTestCheckout {
	t.Helper()
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("git is required:", err)
	}
	testRoot := prepareIntegrationTestRoot(t)
	packageDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	source := strings.TrimSpace(os.Getenv("DISTILLERY_TEST_REPO"))
	if source == "" {
		source = distilleryTestRepositoryURL
	}
	repositoryDirectory := filepath.Join(testRoot, "distillery-test")
	runCommand(t, packageDirectory, gitBinary, "clone", "--quiet", "--no-local", source, repositoryDirectory)
	if expectedHead != "" {
		runCommand(t, repositoryDirectory, gitBinary, "cat-file", "-e", expectedHead+"^{commit}")
		runCommand(t, repositoryDirectory, gitBinary, "checkout", "--quiet", "--detach", expectedHead)
		runCommand(t, repositoryDirectory, gitBinary, "branch", "--force", "main", expectedHead)
		runCommand(t, repositoryDirectory, gitBinary, "checkout", "--quiet", "main")
		head := strings.TrimSpace(runCommand(t, repositoryDirectory, gitBinary, "rev-parse", "HEAD"))
		if head != expectedHead {
			t.Fatalf("distillery-test HEAD is %s, expected fixture revision %s", head, expectedHead)
		}
	}
	return distilleryTestCheckout{
		TestRoot:            testRoot,
		RepositoryDirectory: repositoryDirectory,
		GitBinary:           gitBinary,
	}
}

// Keep the last run's artifacts for debugging; clear only this test's directory
// before starting another run. All integration tests share the repository's out/.
func prepareIntegrationTestRoot(t *testing.T) string {
	t.Helper()
	packageDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repositoryDirectory := packageDirectory
	for {
		if info, err := os.Stat(filepath.Join(repositoryDirectory, "go.mod")); err == nil && !info.IsDir() {
			break
		}
		parent := filepath.Dir(repositoryDirectory)
		if parent == repositoryDirectory {
			t.Fatalf("cannot locate repository root from %s", packageDirectory)
		}
		repositoryDirectory = parent
	}
	testRoot := filepath.Join(repositoryDirectory, "out", filepath.FromSlash(t.Name()))
	assertTestRoot(t, repositoryDirectory, testRoot)
	if err := os.RemoveAll(testRoot); err != nil {
		t.Fatalf("clear test directory: %v", err)
	}
	if err := os.MkdirAll(testRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	return testRoot
}

func assertTestRoot(t *testing.T, repositoryDirectory string, testRoot string) {
	t.Helper()
	relative, err := filepath.Rel(repositoryDirectory, testRoot)
	if err != nil || filepath.ToSlash(relative) != "out/"+t.Name() {
		t.Fatalf("refusing to clear unexpected test path %s", testRoot)
	}
	// Do not follow a redirected out directory while wiping previous results.
	parent := filepath.Dir(testRoot)
	for parent != repositoryDirectory {
		info, err := os.Lstat(parent)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			t.Fatalf("refusing to clear test artifacts through %s", parent)
		}
		parent = filepath.Dir(parent)
	}
}

func TestIntegrationOutputRetainedAndResetBeforeNextRun(t *testing.T) {
	root := prepareIntegrationTestRoot(t)
	// IDE runners may launch from the repository root instead of the package.
	t.Chdir(filepath.Dir(filepath.Dir(root)))
	sibling := filepath.Join(root, "keep")
	if err := os.WriteFile(sibling, []byte("sibling"), 0o600); err != nil {
		t.Fatal(err)
	}
	var retained string
	t.Run("reset", func(t *testing.T) {
		directory := prepareIntegrationTestRoot(t)
		stale := filepath.Join(directory, "stale-state")
		if err := os.WriteFile(stale, []byte("previous run"), 0o600); err != nil {
			t.Fatal(err)
		}
		if next := prepareIntegrationTestRoot(t); next != directory {
			t.Fatalf("test output moved: %s", next)
		}
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Fatalf("previous state was not wiped: %v", err)
		}
		retained = filepath.Join(directory, "last-run.log")
		if err := os.WriteFile(retained, []byte("keep for debugging"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	for _, file := range []string{retained, sibling} {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("test cleanup removed %s: %v", file, err)
		}
	}
}

func runCommand(t *testing.T, directory string, executable string, args ...string) string {
	t.Helper()
	command := exec.Command(executable, args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", executable, strings.Join(args, " "), err, output)
	}
	return string(output)
}
