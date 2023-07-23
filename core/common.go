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
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"

	log "github.com/sirupsen/logrus"
)

// Version returns the version of the Qodana CLI, set during the GoReleaser build
var Version = "dev"

//goland:noinspection GoUnnecessarilyExportedIdentifiers
var (
	QDJVMC = "QDJVMC"
	QDJVM  = "QDJVM"
	QDAND  = "QDAND"
	QDPHP  = "QDPHP"
	QDPY   = "QDPY"
	QDPYC  = "QDPYC"
	QDJS   = "QDJS"
	QDGO   = "QDGO"
	QDNET  = "QDNET"
)

func Image(code string) string {
	switch code {
	case QDAND:
		return "jetbrains/qodana-jvm-android:" + version
	case QDPHP:
		return "jetbrains/qodana-php:" + version
	case QDJS:
		return "jetbrains/qodana-js:" + version
	case QDNET:
		return "jetbrains/qodana-dotnet:" + version
	case QDPY:
		return "jetbrains/qodana-python:" + version
	case QDPYC:
		return "jetbrains/qodana-python-community:" + version
	case QDGO:
		return "jetbrains/qodana-go:" + version
	case QDJVM:
		return "jetbrains/qodana-jvm:" + version
	case QDJVMC:
		return "jetbrains/qodana-jvm-community:" + version
	default:
		log.Fatal("Unknown code: " + code)
		return ""
	}
}

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
	if len(linters) == 0 && !IsInteractive() {
		ErrorMessage("Could not configure project as it is not supported by Qodana")
		WarningMessage("See https://www.jetbrains.com/help/qodana/supported-technologies.html for more details")
		os.Exit(1)
	} else if len(linters) == 1 || !IsInteractive() {
		linter = linters[0]
	} else {
		if len(linters) == 0 {
			linters = allLinters
		}
		choice, err := qodanaInteractiveSelect.WithOptions(linters).Show()
		if err != nil {
			ErrorMessage("%s", err)
			os.Exit(1)
		}
		linter = choice
	}
	if linter != "" {
		log.Infof("Detected linters: %s", strings.Join(linters, ", "))
		SetQodanaLinter(path, linter, yamlName)
	}
	SuccessMessage("Added %s", linter)
	return linter
}

func setupToken(path string, id string) string {
	openCloud := AskUserConfirm("Do you want to open the team page to get the token?")
	if openCloud {
		err := openBrowser(getCloudTeamsPageUrl(path))
		if err != nil {
			ErrorMessage("%s", err)
			return ""
		}
	} else {
		return ""
	}
	token, err := pterm.DefaultInteractiveTextInput.WithMask("*").WithTextStyle(primaryStyle).Show(
		fmt.Sprintf(">  Enter the token (will be saved to the system keyring and used for %s)", PrimaryBold(path)),
	)
	if err != nil {
		ErrorMessage("%s", err)
		return ""
	}
	if token == "" {
		ErrorMessage("Token cannot be empty")
		return ""
	} else {
		err = saveCloudToken(id, token)
		if err != nil {
			ErrorMessage("Failed to save credentials: %s", err)
			return ""
		}
		SuccessMessage("Token was saved to the system keyring")
		return token
	}
}

// ShowReport serves the Qodana report
func ShowReport(cloudUrl string, path string, port int) {
	var url string
	if cloudUrl != "" {
		openReport(url, path, port)
	} else {
		WarningMessage("Press Ctrl+C to stop serving the report\n")
		printProcess(
			func() {
				if _, err := os.Stat(path); os.IsNotExist(err) {
					log.Fatal("Qodana report not found. Get a report by running `qodana scan`")
				}
				openReport("", path, port)
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
