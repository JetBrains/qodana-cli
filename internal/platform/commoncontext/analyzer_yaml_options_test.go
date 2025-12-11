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
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

func TestAnalyzerQodanaYamlOptions(t *testing.T) {
	for _, tt := range optionsTests {
		t.Run(
			tt.name, func(t *testing.T) {
				var fatal bool
				if tt.failure {
					defer func() { logrus.StandardLogger().ExitFunc = nil }()
					logrus.StandardLogger().ExitFunc = func(int) { fatal = true }
				}

				projDir, err := createProjDirWithQodanaYaml(
					qdyaml.QodanaYaml{
						Ide:          tt.ide,
						Linter:       tt.linter,
						WithinDocker: tt.withinDocker,
						Image:        tt.image,
					},
				)
				defer func(path string) {
					_ = os.RemoveAll(path)
				}(projDir)

				if err != nil {
					t.Errorf("%v", err)
				}
				analyzer := getAnalyzerFromProject("token", projDir, "")
				if tt.failure && !fatal {
					t.Errorf("Expected failure case, got %v", analyzer.Name())
					return
				}
				if analyzer == nil {
					if tt.expected != nil {
						t.Errorf("Expected linter to be %s, got nil", tt.expected.Name())
						return
					}
				} else if analyzer.GetLinter() != tt.expected.GetLinter() || analyzer.Name() != tt.expected.Name() {
					t.Errorf("Expected linter to be %s, got %v", tt.expected.Name(), analyzer.Name())
				}
			},
		)
	}
}

func createProjDirWithQodanaYaml(qodanaYaml qdyaml.QodanaYaml) (string, error) {
	dir, err := os.MkdirTemp("", "qodana-yaml-test-*")
	if err != nil {
		return "", err
	}

	yamlData, err := yaml.Marshal(qodanaYaml)
	if err != nil {
		return "", err
	}

	err = os.WriteFile(filepath.Join(dir, "qodana.yaml"), yamlData, 0644)
	if err != nil {
		return "", err
	}

	return dir, nil
}
