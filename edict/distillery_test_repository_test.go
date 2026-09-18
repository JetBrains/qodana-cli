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
	packageDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	testRoot := filepath.Join(packageDirectory, "testtmp", t.Name())
	assertTestRoot(t, packageDirectory, testRoot)
	if err := os.RemoveAll(testRoot); err != nil {
		t.Fatalf("clear test directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(testRoot); err != nil {
			t.Errorf("clear test directory: %v", err)
		}
	})
	if err := os.MkdirAll(testRoot, 0o755); err != nil {
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

func assertTestRoot(t *testing.T, packageDirectory string, testRoot string) {
	t.Helper()
	relative, err := filepath.Rel(packageDirectory, testRoot)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	if len(parts) < 2 || parts[0] != "testtmp" || parts[1] != t.Name() {
		t.Fatalf("refusing to clear unexpected test path %s", testRoot)
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
