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
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/sarif"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

// https://docs.gitlab.com/ee/ci/testing/code_quality.html#implement-a-custom-tool
const (
	// glCodeQualityReport is the name of the GitLab CodeQuality report file
	glCodeQualityReport = "gl-code-quality-report.json"

	//
	codeClimateBlocker  = "blocker"
	codeClimateCritical = "critical"
	codeClimateMajor    = "major"
	codeClimateMinor    = "minor"
	codeClimateInfo     = "info"
)

// toCodeClimateSeverity maps SARIF and Qodana severity levels to Code Climate severity levels
var toCodeClimateSeverity = map[string]string{
	sarifError:     codeClimateCritical,
	sarifWarning:   codeClimateMajor,
	sarifNote:      codeClimateMinor,
	qodanaCritical: codeClimateBlocker,
	qodanaHigh:     codeClimateCritical,
	qodanaModerate: codeClimateMajor,
	qodanaLow:      codeClimateMinor,
	qodanaInfo:     codeClimateInfo,
}

// CCIssue represents a Code Climate (GitLab CodeQuality) issue
type CCIssue struct {
	CheckName   string   `json:"check_name"`
	Description string   `json:"description"`
	Fingerprint string   `json:"fingerprint"`
	Severity    string   `json:"severity"`
	Location    Location `json:"location"`
}

// Location represents a location of the issue
type Location struct {
	Path  string `json:"path"`
	Lines Line   `json:"lines"`
}

// Line represents a line of the issue
type Line struct {
	Begin int `json:"begin"`
}

// sarifResultToCodeClimate converts a SARIF result to a Code Climate issue.
func sarifResultToCodeClimate(r *sarif.Result) CCIssue {
	return CCIssue{
		CheckName:   r.RuleId,
		Description: r.Message.Text,
		Fingerprint: getFingerprint(r),
		Severity:    toCodeClimateSeverity[getSeverity(r)],
		Location: Location{
			Path: r.Locations[0].PhysicalLocation.ArtifactLocation.Uri,
			Lines: Line{
				Begin: int(r.Locations[0].PhysicalLocation.Region.StartLine),
			},
		},
	}
}

// writeGlCodeQualityReport saves GitLab CodeQuality issues to a file in JSON format
func writeGlCodeQualityReport(issues []CCIssue, sarifPath string) error {
	outputFile := filepath.Join(filepath.Dir(sarifPath), glCodeQualityReport)
	file, err := os.Create(outputFile)
	if err != nil {
		log.Warnf("Failed to create GitLab CodeQuality report file: %v", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Warnf("failed to close GitLab CodeQuality report file: %s", err.Error())
		}
	}(file)
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(issues); err != nil {
		return fmt.Errorf("failed to write GitLab CodeQuality report: %w", err)
	}
	return nil
}
