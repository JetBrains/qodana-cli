/*
 * Copyright 2021-2022 JetBrains s.r.o.
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

package core

import (
	"path/filepath"

	"github.com/owenrumney/go-sarif/v2/sarif"
	log "github.com/sirupsen/logrus"
)

const (
	// BaselineStateEmpty default baseline state (not set)
	BaselineStateEmpty = ""
	// BaselineStateNew new baseline state
	BaselineStateNew = "new"
	// BaselineStateUnchanged unchanged baseline state
	BaselineStateUnchanged = "unchanged"
)

// ReadSarif prints Qodana Scan result into stdout
func ReadSarif(sarifPath string, printProblems bool) {
	newProblems := 0
	s, err := sarif.Open(sarifPath)
	if err != nil {
		log.Fatal(err)
	}
	if printProblems {
		EmptyMessage()
	}
	for _, run := range s.Runs {
		for _, r := range run.Results {
			ruleId := *r.RuleID
			message := *r.Message.Text
			level := *r.Level
			baselineState := BaselineStateEmpty
			if r.BaselineState != nil {
				baselineState = *r.BaselineState
			}
			if baselineState == BaselineStateNew || baselineState == BaselineStateEmpty {
				newProblems++
			}
			if printProblems && len(r.Locations) > 0 && baselineState != BaselineStateUnchanged {
				if r.Locations[0].PhysicalLocation != nil {
					startLine := *r.Locations[0].PhysicalLocation.Region.StartLine
					contextLine := *r.Locations[0].PhysicalLocation.ContextRegion.StartLine
					startColumn := *r.Locations[0].PhysicalLocation.Region.StartColumn
					filePath := *r.Locations[0].PhysicalLocation.ArtifactLocation.URI
					context := *r.Locations[0].PhysicalLocation.ContextRegion.Snippet.Text
					printProblem(ruleId, level, message, filePath, startLine, startColumn, contextLine, context)
				} else {
					printProblem(ruleId, level, message, "", 0, 0, 0, "")
				}
			}
		}
	}
	if newProblems == 0 {
		SuccessMessage("It seems all right 👌 No new problems found according to the checks applied")
	} else {
		ErrorMessage("Found %d new problems according to the checks applied", newProblems)
	}
}

// GetReportUrl get Qodana Cloud report URL from the given qodana.sarif.json
func GetReportUrl(resultsDir string) string {
	sarifPath := filepath.Join(resultsDir, QodanaShortSarifName)
	s, err := sarif.Open(sarifPath)
	if err != nil {
		log.Debug(err)
		return ""
	}
	reportUrl, exists := s.Runs[0].Properties["reportUrl"]
	if exists {
		return reportUrl.(string)
	}
	return ""
}
