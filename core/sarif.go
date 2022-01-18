package core

import (
	"fmt"
	"path/filepath"

	"github.com/owenrumney/go-sarif/sarif"
	log "github.com/sirupsen/logrus"
)

// ReadSarif prints Qodana Scan result into stdout
func ReadSarif(resultsDir string, printProblems bool) { // TODO: prepare a summary table
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
