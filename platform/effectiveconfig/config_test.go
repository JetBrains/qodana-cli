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

package effectiveconfig

import (
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuccess(t *testing.T) {
	assert.Equal(t, true, utils.IsInstalled("java"))
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	testCases := []struct {
		testCaseName             string
		localQodanaYaml          string
		globalConfigurationsFile string
		globalConfigurationId    string
	}{
		{
			testCaseName:             "local and global input qodana yaml",
			localQodanaYaml:          "local/qodana.yaml",
			globalConfigurationsFile: "global/qodana-global-configurations.yaml",
			globalConfigurationId:    "main",
		},
		{
			testCaseName:    "only local input qodana yaml",
			localQodanaYaml: "local/qodana.yaml",
		},
		{
			testCaseName:             "only global input qodana yaml",
			globalConfigurationsFile: "global/qodana-global-configurations.yaml",
			globalConfigurationId:    "main",
		},
		{
			testCaseName: "no input qodana yaml",
		},
		{
			testCaseName:    "ide and linter defined in root and child qodana yaml",
			localQodanaYaml: "local/qodana.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(
			tc.testCaseName, func(t *testing.T) {
				testDataPath := filepath.Join(workingDir, "testdata", tc.testCaseName)
				configurationDir := filepath.Join(testDataPath, "configuration")
				if tc.localQodanaYaml != "" {
					tc.localQodanaYaml = filepath.Join(configurationDir, tc.localQodanaYaml)
				}
				if tc.globalConfigurationsFile != "" {
					tc.globalConfigurationsFile = filepath.Join(configurationDir, tc.globalConfigurationsFile)
				}

				systemDir := t.TempDir()
				logDir := t.TempDir()

				configFiles, err := CreateEffectiveConfigFiles(
					tc.localQodanaYaml,
					tc.globalConfigurationsFile,
					tc.globalConfigurationId,
					"java",
					systemDir,
					"qdconfig",
					logDir,
				)
				if err != nil {
					assert.FailNow(t, err.Error())
				}

				verifyDirectoriesContentEqual(t, filepath.Join(testDataPath, "expected"), configFiles.ConfigDir)

				isEmptyConfiguration := tc.globalConfigurationsFile == "" &&
					tc.globalConfigurationId == "" &&
					tc.localQodanaYaml == ""
				if !isEmptyConfiguration {
					effectiveConfig := isFileExists(configFiles.EffectiveQodanaYamlPath)
					assert.True(t, effectiveConfig)
				}
			},
		)
	}
}

func TestError(t *testing.T) {
	assert.Equal(t, true, utils.IsInstalled("java"))
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	testCases := []struct {
		testCaseName             string
		localQodanaYaml          string
		globalConfigurationsFile string
		globalConfigurationId    string
	}{
		{
			testCaseName:             "error no global config id",
			globalConfigurationsFile: "global/qodana-global-configurations.yaml",
		},
		{
			testCaseName:          "error no global config file",
			globalConfigurationId: "main",
		},
		{
			testCaseName:    "error ide and linter defined in inner qodana yaml only",
			localQodanaYaml: "local/qodana.yaml",
		},
		{
			testCaseName:    "error local qodana yaml doesn't exist",
			localQodanaYaml: "local/no-qodana.yaml",
		},
		{
			testCaseName:             "error global configurations file doesn't exist",
			globalConfigurationsFile: "global/no-qodana-global-configurations.yaml",
			globalConfigurationId:    "main",
		},
		{
			testCaseName:             "error global configuration id doesn't exist",
			globalConfigurationsFile: "global/qodana-global-configurations.yaml",
			globalConfigurationId:    "no-main",
		},
		{
			testCaseName:             "error global configuration qodana yaml doesn't exist",
			globalConfigurationsFile: "global/qodana-global-configurations.yaml",
			globalConfigurationId:    "main",
		},
		{
			testCaseName:    "error inner qodana yaml doesnt exist",
			localQodanaYaml: "local/qodana.yaml",
		},
		{
			testCaseName:    "error profile-path doesnt exist",
			localQodanaYaml: "local/qodana.yaml",
		},
		{
			testCaseName:    "error profile-base-path doesnt exist",
			localQodanaYaml: "local/qodana.yaml",
		},
		{
			testCaseName:    "error inner profile doesnt exist",
			localQodanaYaml: "local/qodana.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(
			tc.testCaseName, func(t *testing.T) {
				testDataPath := filepath.Join(workingDir, "testdata", tc.testCaseName)
				configurationDir := filepath.Join(testDataPath, "configuration")
				if tc.localQodanaYaml != "" {
					tc.localQodanaYaml = filepath.Join(configurationDir, tc.localQodanaYaml)
				}
				if tc.globalConfigurationsFile != "" {
					tc.globalConfigurationsFile = filepath.Join(configurationDir, tc.globalConfigurationsFile)
				}

				systemDir := t.TempDir()
				logDir := t.TempDir()

				configFiles, err := CreateEffectiveConfigFiles(
					tc.localQodanaYaml,
					tc.globalConfigurationsFile,
					tc.globalConfigurationId,
					"java",
					systemDir,
					"qdconfig",
					logDir,
				)
				if err == nil {
					assert.FailNow(t, "Expected to fail with error")
				}
				isEmptyConfigFiles := configFiles.ConfigDir == "" &&
					configFiles.EffectiveQodanaYamlPath == "" &&
					configFiles.LocalQodanaYamlPath == "" &&
					configFiles.QodanaConfigJsonPath == ""

				if !isEmptyConfigFiles {
					assert.FailNow(t, "Expected to fail with empty config files, files: %s", configFiles.ConfigDir)
				}
			},
		)
	}
}

func verifyDirectoriesContentEqual(t *testing.T, expectedDir string, actualDir string) {
	if _, err := os.Stat(actualDir); os.IsNotExist(err) {
		t.Fatalf("actualDir does not exist: %s", actualDir)
	}
	// Collect files and directories from a given base directory.
	collectEntries := func(baseDir string) (map[string]os.FileInfo, error) {
		entries := make(map[string]os.FileInfo)
		err := filepath.Walk(
			baseDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(baseDir, path)
				if err != nil {
					return err
				}
				entries[rel] = info
				return nil
			},
		)
		return entries, err
	}

	expectedEntries, err := collectEntries(expectedDir)
	if err != nil {
		t.Error(err)
	}
	actualEntries, err := collectEntries(actualDir)
	if err != nil {
		t.Error(err)
	}

	// Compare expected entries with actual entries.
	for rel, expectedInfo := range expectedEntries {
		if expectedInfo.Name() == ".gitkeep" {
			continue
		}
		actualInfo, ok := actualEntries[rel]
		if !ok {
			t.Errorf("file or directory missing: %s", filepath.Join(actualDir, rel))
		}

		if expectedInfo.IsDir() != actualInfo.IsDir() {
			t.Errorf("mismatched type for: %s", rel)
		}

		// If it's a file, compare its content.
		if !expectedInfo.IsDir() {
			expectedContent, err := os.ReadFile(filepath.Join(expectedDir, rel))
			if err != nil {
				t.Error(err)
			}
			actualContent, err := os.ReadFile(filepath.Join(actualDir, rel))
			if err != nil {
				t.Error(err)
			}
			assert.Equal(
				t,
				systemIndependentLinebreaks(string(expectedContent)),
				systemIndependentLinebreaks(string(actualContent)),
				"mismatched content for: %s",
				rel,
			)
		}
	}

	// Check for unexpected extra entries in actual directory.
	for rel := range actualEntries {
		if _, ok := expectedEntries[rel]; !ok {
			t.Errorf("unexpected file or directory found: %s", rel)
		}
	}
}

func systemIndependentLinebreaks(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	return input
}
