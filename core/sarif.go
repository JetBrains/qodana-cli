/*
 * Copyright 2021-2023 JetBrains s.r.o.
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
	"os"
)

const (
	// baselineStateEmpty default baseline state (not set)
	baselineStateEmpty = ""
	// baselineStateNew new baseline state
	baselineStateNew = "new"
	// baselineStateUnchanged unchanged baseline state
	baselineStateUnchanged = "unchanged"
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
			baselineState := baselineStateEmpty
			if r.BaselineState != nil {
				baselineState = *r.BaselineState
			}
			if baselineState == baselineStateNew || baselineState == baselineStateEmpty {
				newProblems++
			}
			if printProblems && len(r.Locations) > 0 && baselineState != baselineStateUnchanged {
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

func saveSarifProperty(path string, key string, value string) error {
	s, err := sarif.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	if len(s.Runs) > 0 {
		s.Runs[0].AddString(key, value)
	}
	err = os.Remove(path)
	if err != nil {
		log.Fatal(err)
	}
	return s.WriteFile(path)
}
