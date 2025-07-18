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
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
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
