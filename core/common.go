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
	"os"
	"strings"

	"github.com/erikgeiser/promptkit/selection"
	log "github.com/sirupsen/logrus"
)

// QodanaOptions is a struct that contains all the options to run a Qodana linter.
type QodanaOptions struct {
	ResultsDir            string
	CacheDir              string
	ProjectDir            string
	Linter                string
	SourceDirectory       string
	DisableSanity         bool
	ProfileName           string
	ProfilePath           string
	RunPromo              string
	StubProfile           string
	Baseline              string
	BaselineIncludeAbsent bool
	SaveReport            bool
	ShowReport            bool
	Port                  int
	Property              []string
	Script                string
	FailThreshold         string
	Changes               bool
	SendReport            bool
	AnalysisId            string
	Env                   []string
	Volumes               []string
	User                  string
	PrintProblems         bool
	SkipPull              bool
	ClearCache            bool
	YamlName              string
}

var Version = "dev"

// GetLinter gets linter for the given path and saves configName
func GetLinter(path string, yamlName string) string {
	var linters []string
	var linter string
	printProcess(func() {
		languages := readIdeaDir(path)
		if len(languages) == 0 {
			languages, _ = recognizeDirLanguages(path)
		}
		if len(languages) == 0 {
			WarningMessage("No technologies detected (no source code files?)\n")
		} else {
			WarningMessage("Detected technologies: " + strings.Join(languages, ", ") + "\n")
			for _, language := range languages {
				if linter, err := langsLinters[language]; err {
					for _, l := range linter {
						linters = Append(linters, l)
					}
				}
			}
		}
	}, "Scanning project", "")
	if len(linters) == 0 {
		ErrorMessage("Could not configure project as it is not supported by Qodana")
		WarningMessage("See https://www.jetbrains.com/help/qodana/supported-technologies.html for more details")
		os.Exit(1)
	} else if len(linters) == 1 || !IsInteractive() {
		linter = linters[0]
	} else {
		sp := selection.New("Which linter do you want to set up?",
			selection.Choices(linters))
		sp.PageSize = 5
		choice, err := sp.RunPrompt()
		if err != nil {
			ErrorMessage("%s", err)
			os.Exit(1)
		}
		linter = choice.String
	}
	if linter != "" {
		log.Infof("Detected linters: %s", strings.Join(linters, ", "))
		SetQodanaLinter(path, linter, yamlName)
	}
	SuccessMessage("Added %s", linter)
	return linter
}

// ShowReport serves the Qodana report
func ShowReport(path string, port int) { // TODO: Open report from Cloud
	printProcess(
		func() {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				log.Fatal("Qodana report not found. Get the report by running `qodana scan`")
			}
			openReport(path, port)
		},
		fmt.Sprintf("Showing Qodana report at http://localhost:%d ", port),
		"",
	)
}
