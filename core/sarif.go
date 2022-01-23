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
	"fmt"
	"path/filepath"

	"github.com/owenrumney/go-sarif/sarif"
	log "github.com/sirupsen/logrus"
)

// ReadSarif prints Qodana Scan result into stdout
func ReadSarif(resultsDir string, printProblems bool) {
	problems := 0
	s, err := sarif.Open(filepath.Join(resultsDir, "qodana.sarif.json"))
	if err != nil {
		log.Fatal(err)
	}
	if printProblems {
		for _, run := range s.Runs {
			for _, r := range run.Results {
				problems += 1
				ruleId := *r.RuleID
				message := *r.Message.Text
				level := *r.Level
				if len(r.Locations) > 0 {
					startLine := *r.Locations[0].PhysicalLocation.Region.StartLine
					startColumn := *r.Locations[0].PhysicalLocation.Region.StartColumn
					filePath := *r.Locations[0].PhysicalLocation.ArtifactLocation.URI
					PrintLocalizedProblem(ruleId, level, message, filePath, startLine, startColumn)
				} else {
					PrintGlobalProblem(ruleId, level, message)
				}
			}
		}
	} else {
		problems = len(s.Runs[0].Results)
	}
	if problems == 0 {
		SuccessMessage("It seems all right 👌 No problems found according to the checks applied")
	} else {
		ErrorMessage(fmt.Sprintf("Qodana found %d problems according to the checks applied", problems))
	}
}
