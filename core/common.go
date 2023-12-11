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
	"fmt"
	"github.com/JetBrains/qodana-cli/v2023/cloud"
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"

	log "github.com/sirupsen/logrus"
)

// Version returns the version of the Qodana CLI, set during the GoReleaser build
var Version = "dev"
var InterruptChannel chan os.Signal

//goland:noinspection GoUnnecessarilyExportedIdentifiers
var (
	QDJVMC         = "QDJVMC"
	QDJVM          = "QDJVM"
	QDAND          = "QDAND"
	QDPHP          = "QDPHP"
	QDPY           = "QDPY"
	QDPYC          = "QDPYC"
	QDJS           = "QDJS"
	QDGO           = "QDGO"
	QDNET          = "QDNET"
	QDNETC         = "QDNETC"
	QDANDC         = "QDANDC"
	QDRST          = "QDRST"
	QDRUBY         = "QDRUBY"
	QDCL           = "QDCL"
	DockerImageMap = map[string]string{
		QDANDC: "jetbrains/qodana-jvm-android:",
		QDPHP:  "jetbrains/qodana-php:",
		QDJS:   "jetbrains/qodana-js:",
		QDNET:  "jetbrains/qodana-dotnet:",
		QDNETC: "jetbrains/qodana-cdnet:",
		QDPY:   "jetbrains/qodana-python:",
		QDPYC:  "jetbrains/qodana-python-community:",
		QDGO:   "jetbrains/qodana-go:",
		QDJVM:  "jetbrains/qodana-jvm:",
		QDJVMC: "jetbrains/qodana-jvm-community:",
		QDCL:   "jetbrains/qodana-clang:",
		//QDRST:  "jetbrains/qodana-rust:",
	}
)

// AllNativeCodes is a list of all supported Qodana linters product codes
var AllNativeCodes = []string{QDNET}

// support has been disabled now for QDJVMC, QDJVM, QDPHP, QDPY, QDPYC, QDJS, QDGO until further testing

func Image(code string) string {
	if val, ok := DockerImageMap[code]; ok {
		return val + version
	} else {
		log.Fatal("Unknown code: " + code)
		return ""
	}
}

// GetAnalyzer gets linter for the given path and saves configName
func GetAnalyzer(path string, yamlName string) string {
	var analyzers []string
	printProcess(func(_ *pterm.SpinnerPrinter) {
		languages := readIdeaDir(path)
		if len(languages) == 0 {
			languages, _ = recognizeDirLanguages(path)
		}
		if len(languages) == 0 {
			WarningMessage("No technologies detected (no source code files?)\n")
		} else {
			WarningMessage("Detected technologies: " + strings.Join(languages, ", ") + "\n")
			for _, language := range languages {
				if i, err := langsProductCodes[language]; err {
					for _, l := range i {
						analyzers = Append(analyzers, l)
					}
				}
			}
			if len(analyzers) == 0 {
				analyzers = AllCodes
			}
		}
	}, "Scanning project", "")

	selector := func(choices []string) string {
		choice, err := qodanaInteractiveSelect.WithOptions(choices).Show()
		if err != nil {
			ErrorMessage("%s", err)
			return ""
		}
		return choice
	}

	interactive := IsInteractive()
	analyzer := SelectAnalyzer(path, analyzers, interactive, selector)
	if analyzer == "" {
		ErrorMessage("Could not configure project as it is not supported by Qodana")
		WarningMessage("See https://www.jetbrains.com/help/qodana/supported-technologies.html for more details")
		os.Exit(1)
	}
	SetQodanaLinter(path, analyzer, yamlName)
	SuccessMessage("Added %s", analyzer)
	return analyzer
}

func SelectAnalyzer(path string, analyzers []string, interactive bool, selectFunc func([]string) string) string {
	var analyzer string
	if len(analyzers) == 0 && !interactive {
		return ""
	}

	selection, choices := analyzerToSelect(analyzers, path)
	log.Debugf("Detected products: %s", strings.Join(choices, ", "))

	if len(choices) == 1 || !interactive {
		analyzer = selection[choices[0]]
	} else {
		choice := selectFunc(choices)
		if choice == "" {
			return ""
		}
		analyzer = selection[choice]
	}
	return analyzer
}

func IsNativeAnalyzer(analyzer string) bool {
	return Contains(AllNativeCodes, analyzer)
}

func analyzerToSelect(analyzers []string, path string) (map[string]string, []string) {
	analyzersMap := make(map[string]string)
	analyzersList := make([]string, 0, len(analyzers))
	for _, a := range analyzers {
		if IsNativeAnalyzer(a) {
			if IsNativeRequired(path, a) {
				analyzersMap[a+" (Native)"] = a
				analyzersList = append(analyzersList, a+" (Native)")
			}
		}
		analyzersMap[Image(a)+" (Docker)"] = Image(a)
		analyzersList = append(analyzersList, Image(a)+" (Docker)")
	}
	return analyzersMap, analyzersList
}

// ShowReport serves the Qodana report
func ShowReport(resultsDir string, reportPath string, port int) {
	cloudUrl := cloud.GetReportUrl(resultsDir)
	if cloudUrl != "" {
		openReport(cloudUrl, reportPath, port)
	} else {
		WarningMessage("Press Ctrl+C to stop serving the report\n")
		printProcess(
			func(_ *pterm.SpinnerPrinter) {
				if _, err := os.Stat(reportPath); os.IsNotExist(err) {
					log.Fatal("Qodana report not found. Get a report by running `qodana scan`")
				}
				openReport("", reportPath, port)
			},
			fmt.Sprintf("Showing Qodana report from %s", fmt.Sprintf("http://localhost:%d/", port)),
			"",
		)
	}
}

// GetDotNetConfig gets .NET config for the given path and saves configName
func GetDotNetConfig(projectDir string, yamlName string) bool {
	possibleOptions := findFiles(projectDir, []string{".sln", ".csproj", ".vbproj", ".fsproj"})
	if len(possibleOptions) <= 1 {
		return false
	}
	WarningMessage("Detected multiple .NET solution/project files, select the preferred one \n")
	choice, err := qodanaInteractiveSelect.WithOptions(possibleOptions).WithDefaultText("Select solution/project").Show()
	if err != nil {
		ErrorMessage("%s", err)
		return false
	}
	dotnet := &DotNet{}
	if strings.HasSuffix(choice, ".sln") {
		dotnet.Solution = filepath.Base(choice)
	} else {
		dotnet.Project = filepath.Base(choice)
	}
	return setQodanaDotNet(projectDir, dotnet, yamlName)
}
