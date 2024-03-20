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
	"fmt"
	sarif2 "github.com/owenrumney/go-sarif/v2/sarif"
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
						"equalIndicator/v2": "2faa4198335b75139f44d723b5999101424efba41c6caf11e6da4c2d7622ae29"
					},
                    "locations": [
                        {
                            "physicalLocation": {
                                "artifactLocation": {
                                  "uri": "cloud/report.go"
                                },
                                "region": {
                                    "startLine": 62
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
                      "equalIndicator/v2": "3396dd8d145d8bcb1f7a777bc8c49e438e543b9cef8c9e986dcf5bdf5d8f16f5"
					},
                    "locations": [
                        {
                            "physicalLocation": {
                                "artifactLocation": {
                                    "uri": "go.mod"
                                },
                                "region": {
                                    "startLine": 83
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

// Expected CCIssue output
var expectedIssues = []CCIssue{
	{
		CheckName:   "GoUnusedExportedFunction",
		Description: "Unused function 'SaveReportFile'",
		Fingerprint: "2faa4198335b75139f44d723b5999101424efba41c6caf11e6da4c2d7622ae29",
		Severity:    "major",
		Location:    Location{Path: "cloud/report.go", Lines: Line{Begin: 62}},
	},
	{
		CheckName:   "VulnerableLibrariesLocal",
		Description: "Dependency go:golang.org/x/crypto:v0.17.0 is vulnerable, safe version v0.21.0 CVE-2023-42818 9.8 Improper Restriction of Excessive Authentication Attempts vulnerability with High severity found Results powered by Checkmarx(c)",
		Fingerprint: "3396dd8d145d8bcb1f7a777bc8c49e438e543b9cef8c9e986dcf5bdf5d8f16f5",
		Severity:    "critical",
		Location:    Location{Path: "go.mod", Lines: Line{Begin: 83}},
	},
}

func TestSarifResultToCodeClimate(t *testing.T) {
	var sarifReport *sarif2.Report
	sarifReport, _ = sarif2.FromString(sarifFileData)

	for i, r := range sarifReport.Runs[0].Results {
		issue := sarifResultToCodeClimate(r)
		if fmt.Sprintf("%v", issue) != fmt.Sprintf("%v", expectedIssues[i]) {
			t.Errorf("Conversion to CCIssue was incorrect, got: %v, want: %v.\n", issue, expectedIssues[i])
		}
	}
}
