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

	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
)

func TestMount(t *testing.T) {
	linter := mockThirdPartyLinter{}
	tempCacheDir, _ := os.MkdirTemp("", "qodana-platform")
	defer func() {
		_ = os.RemoveAll(tempCacheDir)
	}()

	mountInfo := extractUtils(linter, tempCacheDir)
	if mountInfo.Converter == "" {
		t.Error("extractUtils() failed")
	}

	list := []string{mountInfo.Converter, mountInfo.Fuser, mountInfo.BaselineCli}
	// TODO: should be per-linter test as well
	for _, v := range mountInfo.CustomTools {
		list = append(list, v)
	}

	for _, p := range list {
		_, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				t.Error("Unpacking failed")
			}
		}
	}
}

type mockThirdPartyLinter struct {
}

func (mockThirdPartyLinter) MountTools(_ string) (map[string]string, error) {
	return make(map[string]string), nil
}

func (mockThirdPartyLinter) RunAnalysis(_ thirdpartyscan.Context) error {
	return nil
}

func TestGetToolsMountPath(t *testing.T) {
	dir := t.TempDir()
	path := getToolsMountPath(dir)
	if path == "" {
		t.Error("getToolsMountPath returned empty string")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("getToolsMountPath did not create directory")
	}
}

func TestProcessAuxiliaryTool(t *testing.T) {
	dir := t.TempDir()
	testBytes := []byte("test content")

	path := ProcessAuxiliaryTool("test.jar", "test", dir, testBytes)

	if path == "" {
		t.Error("ProcessAuxiliaryTool returned empty path")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("Failed to read created file: %v", err)
	}

	if string(content) != string(testBytes) {
		t.Error("File content mismatch")
	}

	path2 := ProcessAuxiliaryTool("test.jar", "test", dir, testBytes)
	if path != path2 {
		t.Error("ProcessAuxiliaryTool should return same path on re-run")
	}
}

func TestIsInDirectory(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		target   string
		expected bool
	}{
		{"same dir", "/a/b", "/a/b/file.txt", true},
		{"subdirectory", "/a/b", "/a/b/c/file.txt", true},
		{"different dir", "/a/b", "/a/c/file.txt", false},
		{"parent dir", "/a/b/c", "/a/b/file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInDirectory(tt.base, tt.target)
			if result != tt.expected {
				t.Errorf("isInDirectory(%s, %s) = %v, want %v", tt.base, tt.target, result, tt.expected)
			}
		})
	}
}
