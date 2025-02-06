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
	"github.com/JetBrains/qodana-cli/v2024/platform/platforminit"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func setupTest(projectDir string, fileName string, data string) (*os.File, error) {
	err := os.MkdirAll(projectDir, os.ModePerm)
	if err != nil {
		return nil, err
	}

	file, err := os.Create(projectDir + "/" + fileName)
	if err != nil {
		return nil, err
	}

	_, err = file.WriteString(data)
	if err != nil {
		return nil, err
	}

	err = file.Close()
	return file, err
}

func cleanupTest(projectDir string) error {
	return os.RemoveAll(projectDir)
}

func TestFetchAnalyzerSettings(t *testing.T) {
	t.Run(
		"qodana.yaml exists", func(t *testing.T) {
			projectDir := "./testData/project_with_qodana_yaml"
			expectedIde := "expectedIde"
			fileName := "qodana.yaml"
			data := "ide: " + expectedIde

			_, err := setupTest(projectDir, fileName, data)
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}

			initArgs := platforminit.ComputeArgs(
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				false,
				projectDir,
				"",
			)

			assert.Equal(t, expectedIde, initArgs.Ide)

			if err := cleanupTest(projectDir); err != nil {
				t.Fatal(err)
			}
		},
	)

	t.Run(
		"qodana.yml exists", func(t *testing.T) {
			projectDir := "./testData/project_with_qodana_yml"
			expectedIde := "expectedIde_yml"
			fileName := "qodana.yml"
			data := "ide: " + expectedIde

			_, err := setupTest(projectDir, fileName, data)
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}

			initArgs := platforminit.ComputeArgs(
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				false,
				projectDir,
				"",
			)

			assert.Equal(t, expectedIde, initArgs.Ide)

			if err := cleanupTest(projectDir); err != nil {
				t.Fatal(err)
			}
		},
	)

	t.Run(
		"configName is set", func(t *testing.T) {
			projectDir := "./testData/project_with_custom_qodana_yaml"
			expectedIde := "expectedIde_custom"
			fileName := "custom_qodana.yaml"
			data := "ide: " + expectedIde

			_, err := setupTest(projectDir, fileName, data)
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}

			initArgs := platforminit.ComputeArgs(
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				false,
				projectDir,
				fileName,
			)

			assert.Equal(t, expectedIde, initArgs.Ide)

			if err := cleanupTest(projectDir); err != nil {
				t.Fatal(err)
			}
		},
	)
}
