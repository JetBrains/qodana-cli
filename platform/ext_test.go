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
	"encoding/json"
	bbapi "github.com/reviewdog/go-bitbucket"
	"reflect"
	"testing"
)

// Mock SARIF file data
const sarifFileData = `
{
    "$schema": "https://schemastore.azurewebsites.net/schemas/json/sarif-2.1.0-rtm.4.json",
    "version": "2.1.0",
    "runs": [
        {
            "tool": {
                "driver": {
                    "name": "MockTool",
                    "organization": "MockOrg",
                    "version": "1.0",
                    "rules": [
                        {
                            "id": "GoUnusedExportedFunction",
                            "shortDescription": {
                                "text": "Unused function 'SaveReportFile'"
                            },
                            "properties":{
                                "severity":"major"
                            }
                        },
                        {
                            "id": "VulnerableLibrariesLocal",
                            "shortDescription": {
                                "text": "Dependency go:golang.org/x/crypto:v0.17.0 is vulnerable, safe version v0.21.0 CVE-2023-42818 9.8 Improper Restriction of Excessive Authentication Attempts vulnerability with High severity found Results powered by Checkmarx(c)"
                            },
                            "properties":{
                                "severity":"critical"
                            }
                        },
                        {
                            "id": "ExampleNoteLevel",
                            "shortDescription": {
                                "text": "This is an example note level message."
                            },
                            "properties": {
                                "severity": "medium"
                            }
                        },
                        {
                            "id": "MissingLevel",
                            "shortDescription": {
                                "text": "This result does not specify a level."
                            },
                            "properties": {
                                "severity": "info"
                            }
                        }
                    ]
                }
            },
            "results": [
                {
                    "ruleId": "GoUnusedExportedFunction",
                    "level": "warning",
                    "message": {
                        "text": "Unused function 'SaveReportFile'"
                    },
					"partialFingerprints": {
						"equalIndicator/v2": "2faa123efwsfsdqwer144d723b5999101424efba41c6caf11e6da4c2d7622ae01"
					},
                    "locations": [
                        {
                            "physicalLocation": {
                                "artifactLocation": {
                                  "uri": "src/main/java/AppStarter.java"
                                },
                                "region": {
                                    "startLine": 12
                                }
                            }
                        }
                    ]
                },
                {
                    "ruleId": "VulnerableLibrariesLocal",
                    "level": "error",
                    "message": {
                      "text": "Dependency go:golang.org/x/crypto:v0.17.0 is vulnerable, safe version v0.21.0 CVE-2023-42818 9.8 Improper Restriction of Excessive Authentication Attempts vulnerability with High severity found Results powered by Checkmarx(c)"
                    },
					"partialFingerprints": {
                      "equalIndicator/v2": "2faa123efwsfsdqwer144d723b5999101424efba41c6caf11e6da4c2d7622ae02"
					},
                    "locations": [
                        {
                            "physicalLocation": {
                                "artifactLocation": {
                                    "uri": "src/main/java/AppStarter.java"
                                },
                                "region": {
                                    "startLine": 9
                                }
                            }
                        }
                    ]
                },
                {
                    "ruleId": "ExampleNoteLevel",
                    "level": "note",
                    "message": {
                        "text": "This is an example note level message."
                    },
					"partialFingerprints": {
                      "equalIndicator/v2": "2faa123efwsfsdqwer144d723b5999101424efba41c6caf11e6da4c2d7622ae03"
					},
                    "locations": [
                        {
                            "physicalLocation": {
                                "artifactLocation": {
                                    "uri": "src/main/java/AppStarter.java"
                                },
                                "region": {
                                    "startLine": 2
                                }
                            }
                        }
                    ]
                },
                {
                    "ruleId": "MissingLevel",
                    "level": "info",
                    "message": {
                        "text": "This result does not specify a level."
                    },
					"partialFingerprints": {
                      "equalIndicator/v2": "2faa123efwsfsdqwer144d723b5999101424efba41c6caf11e6da4c2d7622ae04"
					},
                    "locations": [
                        {
                            "physicalLocation": {
                                "artifactLocation": {
                                    "uri": "src/main/java/AppStarter.java"
                                },
                                "region": {
                                    "startLine": 3
                                }
                            }
                        }
                    ]
                }
            ]
        }
    ]
}
`

const expectedAnnotationsAsJSON = `[
  {
    "annotation_type": "CODE_SMELL",
    "details": "This is a long boring description",
    "external_id": "2faa123efwsfsdqwer144d723b5999101424efba41c6caf11e6da4c2d7622ae01",
    "line": 12,
    "path": "src/main/java/AppStarter.java",
    "severity": "MEDIUM",
    "summary": "GoUnusedExportedFunction: Unused function 'SaveReportFile'"
  },
  {
    "annotation_type": "CODE_SMELL",
    "details": "This is a long boring description",
    "external_id": "2faa123efwsfsdqwer144d723b5999101424efba41c6caf11e6da4c2d7622ae02",
    "line": 9,
    "path": "src/main/java/AppStarter.java",
    "severity": "HIGH",
    "summary": "VulnerableLibrariesLocal: Dependency go:golang.org/x/crypto:v0.17.0 is vulnerable, safe version v0.21.0 CVE-2023-42818 9.8 Improper Restriction of Excessive Authentication Attempts vulnerability with High severity found Results powered by Checkmarx(c)"
  },
  {
    "annotation_type": "CODE_SMELL",
    "details": "This is a long boring description",
    "external_id": "2faa123efwsfsdqwer144d723b5999101424efba41c6caf11e6da4c2d7622ae03",
    "line": 2,
    "path": "src/main/java/AppStarter.java",
    "severity": "LOW",
    "summary": "ExampleNoteLevel: This is an example note level message."
  },
  {
    "annotation_type": "CODE_SMELL",
    "details": "This is a long boring description",
    "external_id": "2faa123efwsfsdqwer144d723b5999101424efba41c6caf11e6da4c2d7622ae04",
    "line": 3,
    "path": "src/main/java/AppStarter.java",
    "severity": "LOW",
    "summary": "MissingLevel: This result does not specify a level."
  }
]`

func TestSarifResultToBitBucketAnnotation(t *testing.T) {
	sarifReport, err := ReadReportFromString(sarifFileData)
	if err != nil {
		t.Errorf("Failed to parse SARIF file: %v", err)
	}
	annotations := make([]bbapi.ReportAnnotation, len(sarifReport.Runs[0].Results))
	for i, r := range sarifReport.Runs[0].Results {
		annotations[i] = buildAnnotation(&r, "This is a long boring description", "")
	}
	expectedAnnotations := getExpectedAnnotations(t)
	for i, annotation := range annotations { // doing comparison like this because only some fields are interesting
		if *annotation.Details != *expectedAnnotations[i].Details {
			t.Fatalf("Mismatch in Details at index %d: got %s, want %s\n", i, *annotation.Details, *expectedAnnotations[i].Details)
		}
		if *annotation.AnnotationType != *expectedAnnotations[i].AnnotationType {
			t.Fatalf("Mismatch in AnnotationType at index %d: got %s, want %s\n", i, *annotation.AnnotationType, *expectedAnnotations[i].AnnotationType)
		}
		if *annotation.Summary != *expectedAnnotations[i].Summary {
			t.Fatalf("Mismatch in Summary at index %d: got %s, want %s\n", i, *annotation.Summary, *expectedAnnotations[i].Summary)
		}
		if *annotation.ExternalId != *expectedAnnotations[i].ExternalId {
			t.Fatalf("Mismatch in ExternalId at index %d: got %s, want %s\n", i, *annotation.ExternalId, *expectedAnnotations[i].ExternalId)
		}
		if *annotation.Line != *expectedAnnotations[i].Line {
			t.Fatalf("Mismatch in Line at index %d: got %d, want %d\n", i, *annotation.Line, *expectedAnnotations[i].Line)
		}
		if *annotation.Path != *expectedAnnotations[i].Path {
			t.Fatalf("Mismatch in Path at index %d: got %s, want %s\n", i, *annotation.Path, *expectedAnnotations[i].Path)
		}
		if *annotation.Severity != *expectedAnnotations[i].Severity {
			t.Fatalf("Mismatch in Severity at index %d: got %s, want %s\n", i, *annotation.Severity, *expectedAnnotations[i].Severity)
		}
	}
}

func getExpectedAnnotations(t *testing.T) []bbapi.ReportAnnotation {
	expectedAnnotations := make([]bbapi.ReportAnnotation, 0)
	err := json.Unmarshal([]byte(expectedAnnotationsAsJSON), &expectedAnnotations)
	if err != nil {
		t.Fatalf("Failed to unserialize expected annotations: %v", err)
	}
	return expectedAnnotations
}

// TestSarifResultToCodeClimate tests the conversion of SARIF results to CodeClimate issues.
func TestSarifResultToCodeClimate(t *testing.T) {
	sarifReport, err := ReadReportFromString(sarifFileData)
	if err != nil {
		t.Fatalf("Failed to parse SARIF file: %v", err)
	}

	expectedIssues := []CCIssue{
		{
			CheckName:   "GoUnusedExportedFunction",
			Description: "Unused function 'SaveReportFile'",
			Fingerprint: "2faa123efwsfsdqwer144d723b5999101424efba41c6caf11e6da4c2d7622ae01",
			Severity:    codeClimateMajor,
			Location:    Location{Path: "src/main/java/AppStarter.java", Lines: Line{Begin: 12}},
		},
		{
			CheckName:   "VulnerableLibrariesLocal",
			Description: "Dependency go:golang.org/x/crypto:v0.17.0 is vulnerable, safe version v0.21.0 CVE-2023-42818 9.8 Improper Restriction of Excessive Authentication Attempts vulnerability with High severity found Results powered by Checkmarx(c)",
			Fingerprint: "2faa123efwsfsdqwer144d723b5999101424efba41c6caf11e6da4c2d7622ae02",
			Severity:    codeClimateCritical,
			Location:    Location{Path: "src/main/java/AppStarter.java", Lines: Line{Begin: 9}},
		},
		{
			CheckName:   "ExampleNoteLevel",
			Description: "This is an example note level message.",
			Fingerprint: "2faa123efwsfsdqwer144d723b5999101424efba41c6caf11e6da4c2d7622ae03",
			Severity:    codeClimateMinor, // Based on your mapping logic
			Location:    Location{Path: "src/main/java/AppStarter.java", Lines: Line{Begin: 2}},
		},
		{
			CheckName:   "MissingLevel",
			Description: "This result does not specify a level.",
			Fingerprint: "2faa123efwsfsdqwer144d723b5999101424efba41c6caf11e6da4c2d7622ae04",
			Location:    Location{Path: "src/main/java/AppStarter.java", Lines: Line{Begin: 3}},
		},
	}

	for i, result := range sarifReport.Runs[0].Results {
		issue := sarifResultToCodeClimate(&result)
		if !reflect.DeepEqual(issue, expectedIssues[i]) {
			t.Errorf("Issue at index %d does not match expected. Got %+v, want %+v", i, issue, expectedIssues[i])
		}
	}
}

// Uncomment for local testing
//func TestBitBucketRequest(t *testing.T) {
//	os.Setenv("BITBUCKET_TEST", "true")
//	os.Setenv("BITBUCKET_REPO_FULL_NAME", "tiulpin_/code-analytics-examples")
//	os.Setenv("BITBUCKET_COMMIT", "fa099b0")
//	log.SetLevel(log.DebugLevel)
//	err := sendBitBucketReport(getExpectedAnnotations(t), "Qodana for Everyone", "https://jetbrains.com/qodana/", "qodana-1")
//	if err != nil {
//		t.Errorf("Failed to send BitBucket report: %v", err)
//	}
//}
