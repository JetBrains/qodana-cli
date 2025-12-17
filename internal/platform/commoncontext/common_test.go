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

package commoncontext

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestSelectAnalyzer(t *testing.T) {
	nativePathMaker := func(dir string) error {
		assetsPath := filepath.Join(dir, "Assets")
		projectSettingsPath := filepath.Join(dir, "ProjectSettings")
		_ = os.MkdirAll(assetsPath, os.ModePerm)
		_ = os.MkdirAll(projectSettingsPath, os.ModePerm)
		unityFile := filepath.Join(projectSettingsPath, "ProjectVersion.txt")
		_ = os.WriteFile(unityFile, []byte{}, os.ModePerm)
		return nil
	}
	nonNativePathMaker := func(dir string) error {
		return nil
	}

	tests := []struct {
		name             string
		pathMaker        func(string) error
		analyzers        []product.Linter
		interactive      bool
		selectFunc       func([]string) string
		expectedAnalyzer product.Analyzer
	}{
		{
			name:             "Empty Analyzers Non-interactive",
			pathMaker:        nonNativePathMaker,
			analyzers:        []product.Linter{},
			interactive:      false,
			selectFunc:       nil,
			expectedAnalyzer: nil,
		},
		{
			name:             "Multiple Analyzers Non-interactive",
			pathMaker:        nonNativePathMaker,
			analyzers:        product.AllLinters,
			interactive:      false,
			selectFunc:       nil,
			expectedAnalyzer: product.AllLinters[0].DockerAnalyzer(),
		},
		{
			name:             "Single .NET Analyzer Interactive Non Native",
			pathMaker:        nonNativePathMaker,
			analyzers:        []product.Linter{product.DotNetLinter},
			interactive:      true,
			selectFunc:       func(choices []string) string { return choices[0] },
			expectedAnalyzer: product.DotNetLinter.DockerAnalyzer(),
		},
		{
			name:             "Single .NET Analyzer Interactive Native",
			pathMaker:        nativePathMaker,
			analyzers:        []product.Linter{product.DotNetLinter},
			interactive:      true,
			selectFunc:       func(choices []string) string { return choices[0] },
			expectedAnalyzer: product.DotNetLinter.NativeAnalyzer(),
		},
		{
			name:             "Single .NET Community Analyzer Interactive Native",
			pathMaker:        nativePathMaker,
			analyzers:        []product.Linter{product.DotNetCommunityLinter},
			interactive:      true,
			selectFunc:       func(choices []string) string { return choices[0] },
			expectedAnalyzer: product.DotNetCommunityLinter.DockerAnalyzer(),
		},
		{
			name:             "Multiple Analyzers Interactive",
			pathMaker:        nonNativePathMaker,
			analyzers:        product.AllLinters,
			interactive:      true,
			selectFunc:       func(choices []string) string { return choices[0] },
			expectedAnalyzer: product.AllLinters[0].DockerAnalyzer(),
		},
		{
			name:             "Empty Choice Interactive",
			pathMaker:        nonNativePathMaker,
			analyzers:        product.AllLinters,
			interactive:      true,
			selectFunc:       func(choices []string) string { return "" },
			expectedAnalyzer: nil,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name, func(t *testing.T) {
				dir, err := os.MkdirTemp("", "unity-project")
				if err != nil {
					t.Fatalf("Error creating tmp dir: %v", err)
				}
				defer func(path string) {
					err := os.RemoveAll(path)
					if err != nil {
						t.Fatalf("Error removing tmp dir: %v", err)
					}
				}(dir)
				_ = test.pathMaker(dir)
				got := selectAnalyzer(dir, test.analyzers, test.interactive, test.selectFunc)
				assert.Equal(t, test.expectedAnalyzer, got)
			},
		)
	}
}

func TestReadIdeaDir(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := os.TempDir()
	tempDir = filepath.Join(tempDir, "readIdeaDir")
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(tempDir)

	// Case 1: .idea directory with iml files for Java and Kotlin
	ideaDir := filepath.Join(tempDir, ".idea")
	err := os.MkdirAll(ideaDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	imlFile := filepath.Join(ideaDir, "test.iml")
	err = os.WriteFile(imlFile, []byte("<module type=\"JAVA_MODULE\"/>"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	kotlinImlFile := filepath.Join(ideaDir, "test.kt.iml")
	err = os.WriteFile(kotlinImlFile, []byte("<module type=\"JAVA_MODULE\" languageLevel=\"JDK_1_8\"/>"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	languages := readIdeaDir(tempDir)
	expected := []string{"Java"}
	if !reflect.DeepEqual(languages, expected) {
		t.Errorf("Case 1: Expected %v, but got %v", expected, languages)
	}

	// Case 2: .idea directory with no iml files
	err = os.Remove(imlFile)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Remove(kotlinImlFile)
	if err != nil {
		t.Fatal(err)
	}
	languages = readIdeaDir(tempDir)
	if len(languages) > 0 {
		t.Errorf("Case 1: Expected empty array, but got %v", languages)
	}

	// Case 3: No .idea directory
	err = os.Remove(ideaDir)
	if err != nil {
		t.Fatal(err)
	}
	languages = readIdeaDir(tempDir)
	if len(languages) > 0 {
		t.Errorf("Case 1: Expected empty array, but got %v", languages)
	}
}

func Test_runCmd(t *testing.T) {
	if //goland:noinspection ALL
	runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		for _, tc := range []struct {
			name string
			cmd  []string
			res  int
		}{
			{"true", []string{"true"}, 0},
			{"false", []string{"false"}, 1},
			{"exit 255", []string{"exit 255"}, 255},
		} {
			t.Run(
				tc.name, func(t *testing.T) {
					got, _ := utils.RunCmd("", tc.cmd...)
					if got != tc.res {
						t.Errorf("runCmd: %v, Got: %v, Expected: %v", tc.cmd, got, tc.res)
					}
				},
			)
		}
	}
}

func TestComputeCommonRepositoryRootValidationWithRealFiles(t *testing.T) {
	// Create a temporary directory structure for testing
	tempDir, err := os.MkdirTemp("", "qodana-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp dir: %v", err)
		}
	}()

	// Create subdirectories with lowercase names
	projectDir := filepath.Join(tempDir, "project")
	subDir := filepath.Join(projectDir, "subdir")
	siblingDir := filepath.Join(tempDir, "sibling")

	// Create path variants with different cases for case-sensitivity tests
	// Note: These paths may or may not refer to the same directories depending on OS
	upperTempDir := filepath.Join(tempDir, "PROJECT")        // Different case for project dir
	upperProjectDir := filepath.Join(tempDir, "PROJECT")     // /tmp/xxx/PROJECT
	mixedCaseSubDir := filepath.Join(projectDir, "SubDir")   // /tmp/xxx/project/SubDir
	upperRepoLowerProj := filepath.Join(tempDir, "PROJECT")  // repo=PROJECT, proj=project/subdir
	mixedPathSubDir := filepath.Join(upperTempDir, "subdir") // /tmp/xxx/PROJECT/subdir
	testFile := filepath.Join(tempDir, "testcasesensitive")

	for _, dir := range []string{
		projectDir,
		subDir,
		siblingDir,
		upperProjectDir,
		mixedCaseSubDir,
		mixedPathSubDir,
		testFile,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	// Check if filesystem is case-sensitive
	_, statErr := os.Stat(filepath.Join(tempDir, "testCaseSensitive"))
	isCaseSensitive := os.IsNotExist(statErr)
	t.Logf("Filesystem case-sensitive: %v", isCaseSensitive)

	// Case sensitivity tests: implementation uses os.SameFile for comparison
	// On case-insensitive filesystems (macOS/Windows): paths with different case refer to same dir
	// On case-sensitive filesystems (Linux): paths with different case are different directories
	for _, tc := range []struct {
		name                    string
		projectDir              string
		repositoryRoot          string
		expectRepoEqProj        bool
		failOnCaseSensitiveFS   bool // if true, fails only on case-sensitive filesystems (Linux)
		failOnCaseInsensitiveFS bool // if true, fails only on case-insensitive filesystems (macOS/Windows)
		failAlways              bool // if true, always fails regardless of filesystem
	}{
		// Basic tests
		{
			name:             "RepositoryRoot equals ProjectDir",
			projectDir:       projectDir,
			repositoryRoot:   projectDir,
			expectRepoEqProj: true,
		},
		{
			name:             "RepositoryRoot is parent of ProjectDir",
			projectDir:       subDir,
			repositoryRoot:   projectDir,
			expectRepoEqProj: false,
		},
		{
			name:             "RepositoryRoot empty defaults to ProjectDir",
			projectDir:       projectDir,
			repositoryRoot:   "",
			expectRepoEqProj: true,
		},
		{
			name:           "ProjectDir is sibling of RepositoryRoot - should fail",
			projectDir:     projectDir,
			repositoryRoot: siblingDir,
			failAlways:     true,
		},
		{
			name:           "ProjectDir is parent of RepositoryRoot - should fail",
			projectDir:     projectDir,
			repositoryRoot: subDir,
			failAlways:     true,
		},
		// Case sensitivity tests - implementation uses os.SameFile
		// On case-insensitive FS (macOS/Windows): succeeds (same directory)
		// On case-sensitive FS (Linux): fails (different directories)
		{
			name:                  "Case: RepositoryRoot uppercase, ProjectDir lowercase (same dir different case)",
			projectDir:            projectDir,      // /tmp/xxx/project
			repositoryRoot:        upperProjectDir, // /tmp/xxx/PROJECT
			expectRepoEqProj:      true,
			failOnCaseSensitiveFS: true, // Fails on Linux (different directories)
		},
		{
			name:             "Case: SubDir with mixed case in child path (parent matches exactly)",
			projectDir:       mixedCaseSubDir, // /tmp/xxx/project/SubDir
			repositoryRoot:   projectDir,      // /tmp/xxx/project
			expectRepoEqProj: false,           // Different paths, but project is parent
		},
		{
			name:                  "Case: Uppercase repo with lowercase project subdir",
			projectDir:            subDir,             // /tmp/xxx/project/subdir
			repositoryRoot:        upperRepoLowerProj, // /tmp/xxx/PROJECT
			expectRepoEqProj:      false,
			failOnCaseSensitiveFS: true, // Fails on Linux (PROJECT != project)
		},
		{
			name:                  "Case: Mixed case path - uppercase parent, lowercase child",
			projectDir:            mixedPathSubDir, // /tmp/xxx/PROJECT/subdir
			repositoryRoot:        projectDir,      // /tmp/xxx/project
			expectRepoEqProj:      false,
			failOnCaseSensitiveFS: true, // Fails on Linux (PROJECT != project)
		},
	} {
		t.Run(
			tc.name, func(t *testing.T) {
				// Determine if this test case should fail based on filesystem type
				shouldFail := tc.failAlways ||
					(tc.failOnCaseSensitiveFS && isCaseSensitive) ||
					(tc.failOnCaseInsensitiveFS && !isCaseSensitive)

				var fatal bool
				defer func() { logrus.StandardLogger().ExitFunc = nil }()
				logrus.StandardLogger().ExitFunc = func(int) { fatal = true }

				ctx := computeCommon(
					product.JvmLinter.DockerAnalyzer(),
					tc.projectDir,
					tc.repositoryRoot,
					"",
					"",
					"",
					false,
					"",
				)

				if shouldFail && !fatal {
					t.Errorf("Expected fatal error but got ctx: %v", ctx)
					return
				}

				if shouldFail && fatal {
					return
				}

				if !shouldFail && fatal {
					t.Errorf("Expected success but got fatal error")
					return
				}

				if tc.expectRepoEqProj {
					if ctx.RepositoryRoot != ctx.ProjectDir {
						t.Errorf(
							"Expected RepositoryRoot to equal ProjectDir. Got RepositoryRoot=%s, ProjectDir=%s",
							ctx.RepositoryRoot, ctx.ProjectDir,
						)
					}
				} else {
					if ctx.RepositoryRoot == ctx.ProjectDir {
						t.Errorf("Expected RepositoryRoot to differ from ProjectDir. Got both=%s", ctx.RepositoryRoot)
					}
				}
			},
		)
	}
}
