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
	"path/filepath"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	"github.com/JetBrains/qodana-cli/internal/sarif"
	"github.com/stretchr/testify/assert"
)

func TestMergeSarifReports(t *testing.T) {
	if err := os.Setenv("QODANA_AUTOMATION_GUID", "00000000-0000-1000-8000-000000000000"); err != nil {
		t.Fail()
	}
	if err := os.Setenv("QODANA_REPORT_ID", "43210"); err != nil {
		t.Fail()
	}
	if err := os.Setenv("QODANA_JOB_URL", "joburl"); err != nil {
		t.Fail()
	}
	if err := os.Setenv("QODANA_REMOTE_URL", "repourl"); err != nil {
		t.Fail()
	}
	if err := os.Setenv("QODANA_BRANCH", "foo"); err != nil {
		t.Fail()
	}
	if err := os.Setenv("QODANA_REVISION", "bar"); err != nil {
		t.Fail()
	}
	toolCode := "QDCLC"
	toolDesc := "Qodana for C/C++ (CMake)"
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	testdataPath := filepath.Join(workingDir, "testdata")
	// create temp directory
	dir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(dir)
	err = os.Mkdir(filepath.Join(dir, "tmp"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = utils.CopyDir(filepath.Join(testdataPath, "merge"), filepath.Join(dir, "tmp"))
	if err != nil {
		t.Fatal(err)
	}

	context := thirdpartyscan.ContextBuilder{
		ProjectDir: dir,
		ResultsDir: dir,
		LinterInfo: thirdpartyscan.LinterInfo{
			ProductCode:           toolCode,
			LinterPresentableName: toolDesc,
			LinterVersion:         "",
		},
	}.Build()

	_, err = MergeSarifReports(context, "01234")
	if err != nil {
		t.Fatal(err)
	}
	// check if file exists
	_, err = os.Stat(filepath.Join(dir, "qodana.sarif.json"))
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatal("Resulting SARIF file not found")
		}
	}
	actual, err := os.ReadFile(filepath.Join(dir, "qodana.sarif.json"))
	if err != nil {
		t.Fatal(err)
	}
	// compare with expected file
	expected, err := os.ReadFile(filepath.Join(testdataPath, "merged.qodana.sarif.json"))
	if err != nil {
		t.Fatal(err)
	}
	envs := []string{
		"QODANA_AUTOMATION_GUID",
		"QODANA_REPORT_ID",
		"QODANA_JOB_URL",
		"QODANA_REMOTE_URL",
		"QODANA_BRANCH",
		"QODANA_REVISION",
	}

	for _, env := range envs {
		if err := os.Unsetenv(env); err != nil {
			t.Fatalf("Failed to unset environment variable %s: %v", env, err)
		}
	}
	// do comparison
	expString := normalize(string(expected))
	actString := normalize(string(actual))

	if expString != actString {
		t.Fatalf("Files are not of equal. Length: expected %d vs actual %d", len(expString), len(actString))
	}
}

func normalize(s string) string {
	return strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(s)
}

func TestMakeShortSarif(t *testing.T) {
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	testdataPath := filepath.Join(workingDir, "testdata", "short-sarif")
	sarifPath := filepath.Join(testdataPath, "qodana.sarif.json")
	expectedShortSarifPath := filepath.Join(testdataPath, "qodana-short.sarif.json")

	// Create temp directory for output
	dir, err := os.MkdirTemp("", "test-short-sarif")
	if err != nil {
		t.Fatal(err)
	}
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(dir)

	outputPath := filepath.Join(dir, "output-short.sarif.json")

	err = MakeShortSarif(sarifPath, outputPath)
	if err != nil {
		t.Fatal(err)
	}

	actual, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	expected, err := os.ReadFile(expectedShortSarifPath)
	if err != nil {
		t.Fatal(err)
	}

	expString := normalize(string(expected))
	actString := normalize(string(actual))

	if expString != actString {
		t.Fatalf("Files are not equal. Length: expected %d vs actual %d", len(expString), len(actString))
	}
}

func TestPrintSarifProblem(t *testing.T) {
	t.Run("Happy path", func(t *testing.T) {
		assert.NoError(t, printSarifProblem(&sarif.Result{
			Locations: []sarif.Location{
				{
					PhysicalLocation: &sarif.PhysicalLocation{
						ArtifactLocation: &sarif.ArtifactLocation{
							Uri: "example.cpp",
						},
						Region: &sarif.Region{
							StartLine:   15,
							StartColumn: 1,
						},
						ContextRegion: &sarif.Region{
							StartLine: 10,
							Snippet: &sarif.ArtifactContent{
								Text: "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod\n" +
									"tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim\n" +
									"veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea\n" +
									"commodo consequat. Duis aute irure dolor in reprehenderit in\n" +
									"voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint\n" +
									"occaecat cupidatat non proident, sunt in culpa qui officia deserunt\n" +
									"mollit anim id est laborum.",
							},
						},
					},
				},
			},
		}, "", ""))
	})

	t.Run("Result is nil", func(t *testing.T) {
		assert.Error(t, printSarifProblem(nil, "", ""))
	})
	t.Run("Result is empty", func(t *testing.T) {
		assert.NoError(t, printSarifProblem(&sarif.Result{}, "", ""))
	})
	t.Run("Result contains no physical locations", func(t *testing.T) {
		assert.NoError(t, printSarifProblem(&sarif.Result{
			Locations: []sarif.Location{
				{},
			},
		}, "", ""))
	})
	t.Run("Physical location is empty", func(t *testing.T) {
		assert.NoError(t, printSarifProblem(&sarif.Result{
			Locations: []sarif.Location{
				{
					PhysicalLocation: &sarif.PhysicalLocation{},
				},
			},
		}, "", ""))
	})
	t.Run("Context region contains no snippet", func(t *testing.T) {
		assert.NoError(t, printSarifProblem(&sarif.Result{
			Locations: []sarif.Location{
				{
					PhysicalLocation: &sarif.PhysicalLocation{
						ContextRegion: &sarif.Region{},
					},
				},
			},
		}, "", ""))
	})
}

func TestRunGUID(t *testing.T) {
	t.Run("from env", func(t *testing.T) {
		_ = os.Setenv("QODANA_AUTOMATION_GUID", "test-guid-123")
		defer func() {
			_ = os.Unsetenv("QODANA_AUTOMATION_GUID")
		}()
		assert.Equal(t, "test-guid-123", RunGUID())
	})

	t.Run("generated when not set", func(t *testing.T) {
		_ = os.Unsetenv("QODANA_AUTOMATION_GUID")
		guid := RunGUID()
		assert.NotEmpty(t, guid)
		assert.Len(t, guid, 36)
	})
}

func TestReportId(t *testing.T) {
	t.Run("from env", func(t *testing.T) {
		_ = os.Setenv("QODANA_REPORT_ID", "custom-report-id")
		defer func() {
			_ = os.Unsetenv("QODANA_REPORT_ID")
		}()
		assert.Equal(t, "custom-report-id", ReportId("project"))
	})

	t.Run("generated from project", func(t *testing.T) {
		_ = os.Unsetenv("QODANA_REPORT_ID")
		_ = os.Unsetenv("QODANA_PROJECT_ID")
		id := ReportId("myproject")
		assert.Contains(t, id, "myproject/qodana/")
	})

	t.Run("generated from project id env", func(t *testing.T) {
		_ = os.Unsetenv("QODANA_REPORT_ID")
		_ = os.Setenv("QODANA_PROJECT_ID", "env-project")
		defer func() {
			_ = os.Unsetenv("QODANA_PROJECT_ID")
		}()
		id := ReportId("ignored")
		assert.Contains(t, id, "env-project/qodana/")
	})
}

func TestJobUrl(t *testing.T) {
	t.Run("from env", func(t *testing.T) {
		_ = os.Setenv("QODANA_JOB_URL", "https://example.com/job/123")
		defer func() {
			_ = os.Unsetenv("QODANA_JOB_URL")
		}()
		assert.Equal(t, "https://example.com/job/123", JobUrl())
	})

	t.Run("empty when not set", func(t *testing.T) {
		_ = os.Unsetenv("QODANA_JOB_URL")
		assert.Equal(t, "", JobUrl())
	})
}

func TestGetSeverity(t *testing.T) {
	tests := []struct {
		name     string
		result   *sarif.Result
		expected string
	}{
		{
			name: "from qodanaSeverity property",
			result: &sarif.Result{
				Properties: &sarif.PropertyBag{
					AdditionalProperties: map[string]any{
						"qodanaSeverity": "Critical",
					},
				},
			},
			expected: "Critical",
		},
		{
			name: "from level when no properties",
			result: &sarif.Result{
				Level: "error",
			},
			expected: "error",
		},
		{
			name:     "default note when nothing set",
			result:   &sarif.Result{},
			expected: "note",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, getSeverity(tt.result))
		})
	}
}

func TestRemoveDuplicates(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := removeDuplicates([]sarif.Result{})
		assert.Empty(t, result)
	})

	t.Run("no fingerprints", func(t *testing.T) {
		results := []sarif.Result{
			{RuleId: "rule1"},
			{RuleId: "rule2"},
		}
		filtered := removeDuplicates(results)
		assert.Len(t, filtered, 2)
	})

	t.Run("removes duplicates by fingerprint", func(t *testing.T) {
		results := []sarif.Result{
			{
				RuleId:              "rule1",
				PartialFingerprints: map[string]string{"equalIndicator/v2": "fp1"},
			},
			{
				RuleId:              "rule1",
				PartialFingerprints: map[string]string{"equalIndicator/v2": "fp1"},
			},
			{
				RuleId:              "rule2",
				PartialFingerprints: map[string]string{"equalIndicator/v2": "fp2"},
			},
		}
		filtered := removeDuplicates(results)
		assert.Len(t, filtered, 2)
	})
}

func TestGetFingerprint(t *testing.T) {
	t.Run("v2 fingerprint", func(t *testing.T) {
		r := &sarif.Result{
			PartialFingerprints: map[string]string{
				"equalIndicator/v2": "fingerprint-v2",
				"equalIndicator/v1": "fingerprint-v1",
			},
		}
		assert.Equal(t, "fingerprint-v2", getFingerprint(r))
	})

	t.Run("v1 fingerprint fallback", func(t *testing.T) {
		r := &sarif.Result{
			PartialFingerprints: map[string]string{
				"equalIndicator/v1": "fingerprint-v1",
			},
		}
		assert.Equal(t, "fingerprint-v1", getFingerprint(r))
	})
}

func TestFindSarifFiles(t *testing.T) {
	dir, err := os.MkdirTemp("", "sarif-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	if err := os.WriteFile(filepath.Join(dir, "report1.sarif.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report2.SARIF.JSON"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notasarif.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := findSarifFiles(dir)
	assert.NoError(t, err)
	assert.Len(t, files, 2)
}

func TestReadReportFromString(t *testing.T) {
	sarifJson := `{"$schema":"https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json","version":"2.1.0","runs":[]}`

	report, err := ReadReportFromString(sarifJson)
	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, "2.1.0", report.Version)
}

func TestWriteReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-report.sarif.json")

	report := &sarif.Report{
		Version: "2.1.0",
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Runs:    []sarif.Run{},
	}

	err := WriteReport(path, report)
	assert.NoError(t, err)

	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestReadReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sarif.json")

	sarifContent := `{"$schema":"https://schema.json","version":"2.1.0","runs":[]}`
	if err := os.WriteFile(path, []byte(sarifContent), 0644); err != nil {
		t.Fatal(err)
	}

	report, err := ReadReport(path)
	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, "2.1.0", report.Version)
}

func TestMakeShortSarif_Additional(t *testing.T) {
	dir := t.TempDir()
	sarifPath := filepath.Join(dir, "qodana.sarif.json")
	shortPath := filepath.Join(dir, "qodana-short.sarif.json")

	sarifContent := `{
		"$schema":"https://schema.json",
		"version":"2.1.0",
		"runs":[{
			"tool":{"driver":{"name":"test"}},
			"results":[],
			"invocations":[{"executionSuccessful":true}]
		}]
	}`
	if err := os.WriteFile(sarifPath, []byte(sarifContent), 0644); err != nil {
		t.Fatal(err)
	}

	err := MakeShortSarif(sarifPath, shortPath)
	assert.NoError(t, err)

	_, err = os.Stat(shortPath)
	assert.NoError(t, err)
}

func TestGetShortSarifPath(t *testing.T) {
	path := GetShortSarifPath("/results")
	assert.Contains(t, path, "qodana-short.sarif.json")
}

func TestGetSarifPath(t *testing.T) {
	path := GetSarifPath("/results")
	assert.Contains(t, path, "qodana.sarif.json")
}

func TestGetRuleDescription(t *testing.T) {
	report := &sarif.Report{
		Runs: []sarif.Run{
			{
				Tool: &sarif.Tool{
					Extensions: []sarif.ToolComponent{
						{
							Rules: []sarif.ReportingDescriptor{
								{
									Id: "TEST001",
									ShortDescription: &sarif.MultiformatMessageString{
										Text: "Test rule description",
									},
								},
								{
									Id: "TEST002",
									ShortDescription: &sarif.MultiformatMessageString{
										Text: "Another rule",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	t.Run("existing rule", func(t *testing.T) {
		desc := getRuleDescription(report, "TEST001")
		assert.Equal(t, "Test rule description", desc)
	})

	t.Run("another existing rule", func(t *testing.T) {
		desc := getRuleDescription(report, "TEST002")
		assert.Equal(t, "Another rule", desc)
	})

	t.Run("non-existent rule", func(t *testing.T) {
		desc := getRuleDescription(report, "NONEXISTENT")
		assert.Empty(t, desc)
	})

	t.Run("empty report", func(t *testing.T) {
		emptyReport := &sarif.Report{}
		desc := getRuleDescription(emptyReport, "TEST001")
		assert.Empty(t, desc)
	})
}
