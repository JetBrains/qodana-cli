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
	"github.com/owenrumney/go-sarif/v2/sarif"
	log "github.com/sirupsen/logrus"
)

// ReadSarif prints Qodana Scan result into stdout
func ReadSarif(sarifPath string, printProblems bool) {
	problems := 0
	s, err := sarif.Open(sarifPath)
	if err != nil {
		log.Fatal(err)
	}
	problems = len(s.Runs[0].Results)
	if printProblems {
		EmptyMessage()
		for _, run := range s.Runs {
			for _, r := range run.Results {
				ruleId := *r.RuleID
				message := *r.Message.Text
				level := *r.Level
				if len(r.Locations) > 0 {
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
	}
	if problems == 0 {
		SuccessMessage("It seems all right 👌 No problems found according to the checks applied")
	} else {
		ErrorMessage("Found %d problems according to the checks applied", problems)
	}
}

// GetReportUrl get Qodana Cloud report URL from the given qodana.sarif.json
func GetReportUrl(sarifPath string) string {
	s, err := sarif.Open(sarifPath)
	if err != nil {
		log.Fatal(err)
	}
	reportUrl, exists := s.Runs[0].Properties["reportUrl"]
	if exists {
		return reportUrl.(string)
	}
	return ""
}
