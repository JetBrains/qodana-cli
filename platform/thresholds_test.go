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
	"github.com/JetBrains/qodana-cli/v2024/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/v2024/platform/thirdpartyscan"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestFailureThresholds(t *testing.T) {
	for _, testData := range []struct {
		name     string
		yaml     string
		option   string
		expected string
	}{
		{
			name:     "empty",
			yaml:     "",
			option:   "",
			expected: "",
		},
		{
			name:     "failThreshold set to 0",
			yaml:     `failThreshold: 0`,
			option:   "",
			expected: " --threshold-any=0",
		},
		{
			name: "multiple thresholds",
			yaml: `failureConditions:
  severityThresholds:
    any: 1
    critical: 2
    high: 3
    moderate: 4
    low: 5
    info: 6
`,
			option:   "",
			expected: " --threshold-any=1 --threshold-critical=2 --threshold-high=3 --threshold-info=6 --threshold-low=5 --threshold-moderate=4",
		},
		{
			name: "multiple thresholds overlapping failThreshold",
			yaml: `failureConditions:
  severityThresholds:
    any: 1
    critical: 2
    high: 3
    moderate: 4
    low: 5
    info: 6
failThreshold: 123
`,
			option:   "",
			expected: " --threshold-any=1 --threshold-critical=2 --threshold-high=3 --threshold-info=6 --threshold-low=5 --threshold-moderate=4",
		},
		{
			name: "multiple thresholds non-overlapping failThreshold",
			yaml: `failureConditions:
  severityThresholds:
    critical: 2
    high: 3
    moderate: 4
    low: 5
    info: 6
failThreshold: 123
`,
			option:   "",
			expected: " --threshold-any=123 --threshold-critical=2 --threshold-high=3 --threshold-info=6 --threshold-low=5 --threshold-moderate=4",
		},
		{
			name: "cli option ovevrrides yaml settings",
			yaml: `failureConditions:
  severityThresholds:
    any: 1
    critical: 2
    high: 3
    moderate: 4
    low: 5
    info: 6
`,
			option:   "123",
			expected: " --threshold-any=123",
		},
	} {
		t.Run(
			testData.name, func(t *testing.T) {
				tempDir := t.TempDir()
				// create qodana.yaml if needed
				if testData.yaml != "" {
					if err := os.WriteFile(
						filepath.Join(tempDir, "qodana.yaml"),
						[]byte(testData.yaml),
						0o644,
					); err != nil {
						t.Fatal(err)
					}
				}
				yaml := qdyaml.TestOnlyLoadLocalNotEffectiveQodanaYaml(tempDir, "qodana.yaml")
				c := thirdpartyscan.ContextBuilder{
					FailThreshold:    testData.option,
					QodanaYamlConfig: thirdpartyscan.YamlConfig(yaml),
				}.Build()
				thresholds := getFailureThresholds(c)
				thresholdArgs := thresholdsToArgs(thresholds)
				sort.Strings(thresholdArgs)
				argString := ""
				for _, arg := range thresholdArgs {
					argString += " " + arg
				}

				if argString != testData.expected {
					t.Errorf("expected argString to be '%s' got '%s'", testData.expected, argString)
				}
			},
		)
	}
}
