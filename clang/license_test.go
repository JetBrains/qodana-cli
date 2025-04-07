package main

import (
	"github.com/JetBrains/qodana-cli/v2025/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"
	"os"
	"path/filepath"
	"testing"
)

func TestAllowedChecksByLicenseAndYaml(t *testing.T) {
	for _, testData := range []struct {
		name     string
		plan     string
		yaml     string
		expected string
		isEap    bool
	}{
		{
			name:     "community",
			plan:     "COMMUNITY",
			yaml:     "",
			expected: "--checks=-clion-*",
			isEap:    false,
		},
		{
			name:     "community",
			plan:     "COMMUNITY",
			yaml:     "",
			expected: "",
			isEap:    true,
		},
		{
			name:     "ultimate",
			plan:     "ULTIMATE",
			yaml:     "",
			expected: "",
			isEap:    false,
		},
		{
			name:     "ultimate plus",
			plan:     "ULTIMATE_PLUS",
			yaml:     "",
			expected: "",
			isEap:    false,
		},
		{
			name:     "trial ultimate",
			plan:     "TRIAL_ULTIMATE",
			yaml:     "",
			expected: "",
			isEap:    false,
		},
		{
			name:     "trial ultimate plus",
			plan:     "TRIAL_ULTIMATE_PLUS",
			yaml:     "",
			expected: "",
			isEap:    false,
		},
		{
			name: "community with yaml",
			plan: "COMMUNITY",
			yaml: `version: "1.0"
profile:
  name: qodana.starter
exclude:
  - name: modernize-return-braced-init-list
`,
			expected: "--checks=-clion-*,-modernize-return-braced-init-list",
			isEap:    false,
		},
		{
			name: "ultimate with yaml",
			plan: "ULTIMATE",
			yaml: `version: "1.0"
profile:
  name: qodana.starter
exclude:
  - name: modernize-return-braced-init-list
`,
			expected: "--checks=-modernize-return-braced-init-list",
			isEap:    false,
		},
		{
			name: "community with yaml and MISRA",
			plan: "COMMUNITY",
			yaml: `version: "1.0"
profile:
  name: qodana.starter
include:
  - name: clion-misra-cpp2008-*
exclude:
  - name: modernize-return-braced-init-list
`,
			expected: "--checks=-clion-*,-modernize-return-braced-init-list",
			isEap:    false,
		},
		{
			name: "ultimate with yaml",
			plan: "ULTIMATE",
			yaml: `version: "1.0"
profile:
  name: qodana.starter
include:
  - name: clion-misra-cpp2008-*
exclude:
  - name: modernize-return-braced-init-list
`,
			expected: "--checks=clion-misra-cpp2008-*,-modernize-return-braced-init-list",
			isEap:    false,
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

				context := thirdpartyscan.ContextBuilder{
					ProjectDir: tempDir,
					CloudData: thirdpartyscan.ThirdPartyStartupCloudData{
						LicensePlan: testData.plan,
					},
					LinterInfo: thirdpartyscan.LinterInfo{
						IsEap: testData.isEap,
					},
					QodanaYamlConfig: thirdpartyscan.YamlConfig(yaml),
				}.Build()

				checks, _ := allowedChecksByLicenseAndYaml(context)
				if checks != testData.expected {
					t.Errorf("expected checks to be '%s' got '%s'", testData.expected, checks)
				}
			},
		)
	}
}
