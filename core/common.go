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
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/erikgeiser/promptkit/selection"
	"github.com/pterm/pterm"

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
	Property              string
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
}

var (
	unofficialLinter      = false
	Version               = "1.0.1"
	Interrupted           = false
	SkipCheckForUpdateEnv = "QODANA_CLI_SKIP_CHECK_FOR_UPDATE"
	scanStages            = []string{
		"Preparing Qodana Docker images",
		"Starting the analysis engine",
		"Opening the project",
		"Configuring the project",
		"Analyzing the project",
		"Preparing the report",
	}
	notSupportedLinters = []string{
		"jetbrains/qodana-clone-finder",
	}
	releaseUrl = "https://api.github.com/repos/JetBrains/qodana-cli/releases/latest"
)

// GetLinter gets linter for the given path
func GetLinter(path string) string {
	var linters []string
	var linter string
	PrintProcess(func() {
		languages := readIdeaDir(path)
		if len(languages) == 0 {
			languages, _ = recognizeDirLanguages(path)
		}
		WarningMessage("Detected technologies: " + strings.Join(languages, ", ") + "\n")
		for _, language := range languages {
			if linter, err := langsLinters[language]; err {
				for _, l := range linter {
					if !contains(linters, l) {
						linters = append(linters, l)
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
		WriteQodanaYaml(path, linter)
	}
	SuccessMessage("Added %s", linter)
	return linter
}

// CheckLinter validates the image used for the scan.
func CheckLinter(image string) {
	if !strings.HasPrefix(image, OfficialDockerPrefix) {
		unofficialLinter = true
	}
	for _, linter := range notSupportedLinters {
		if linter == image {
			log.Fatalf("%s is not supported by Qodana CLI", linter)
		}
	}
}

// PrepareHost cleans up report folder, gets the current user, creates the necessary folders for the analysis.
func PrepareHost(opts *QodanaOptions) {
	linterHome := GetLinterSystemDir(opts.ProjectDir, opts.Linter)
	if opts.ResultsDir == "" {
		opts.ResultsDir = filepath.Join(linterHome, "results")
	}
	if opts.CacheDir == "" {
		opts.CacheDir = filepath.Join(linterHome, "cache")
	}
	if opts.User == "" {
		opts.User = fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	}
	if _, err := os.Stat(opts.ResultsDir); err == nil {
		err := os.RemoveAll(opts.ResultsDir)
		if err != nil {
			return
		}
	}
	if err := os.MkdirAll(opts.CacheDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	if err := os.MkdirAll(opts.ResultsDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
}

// ShowReport serves the Qodana report
func ShowReport(path string, port int) { // TODO: Open report from Cloud
	PrintProcess(
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

// RunLinter runs the linter with the given options.
func RunLinter(ctx context.Context, options *QodanaOptions) int {
	docker := getDockerClient()
	for i, stage := range scanStages {
		scanStages[i] = PrimaryBold("[%d/%d] ", i+1, len(scanStages)+1) + Primary(stage)
	}
	CheckLinter(options.Linter)
	if unofficialLinter {
		WarningMessage("You are using an unofficial Qodana linter: %s\n", options.Linter)
	}
	progress, _ := startQodanaSpinner(scanStages[0])
	if !(options.SkipPull) {
		PullImage(ctx, docker, options.Linter)
	}
	dockerConfig := getDockerOptions(options)
	updateText(progress, scanStages[1])
	runContainer(ctx, docker, dockerConfig)
	logs, err := docker.ContainerLogs(ctx, dockerConfig.Name, dockerLogsOptions)
	if err != nil {
		log.Fatal(err)
	}
	defer func(reader io.ReadCloser) {
		err := reader.Close()
		if err != nil {
			log.Fatal(err.Error())
		}
	}(logs)
	followLinter(logs, progress)
	exitCode := getDockerExitCode(ctx, docker, dockerConfig.Name)
	if progress != nil {
		_ = progress.Stop()
	}
	return int(exitCode)
}

// followLinter follows the linter logs and prints the progress.
func followLinter(logs io.ReadCloser, progress *pterm.SpinnerPrinter) {
	reader := bufio.NewReader(logs)
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSuffix(line, "\n")
		if err == nil || len(line) > 0 {
			if strings.Contains(line, "Starting up") {
				updateText(progress, scanStages[2])
			}
			if strings.Contains(line, "The Project opening stage completed in") {
				updateText(progress, scanStages[3])
			}
			if strings.Contains(line, "The Project configuration stage completed in") {
				updateText(progress, scanStages[4])
			}
			if strings.Contains(line, "Detailed summary") {
				updateText(progress, scanStages[5])
				if !IsInteractive() {
					EmptyMessage()
				}
			}
			if strings.Contains(line, "IDEA exit code:") {
				return
			}
			printLinterLog(line)
		}
		if err != nil {
			if err != io.EOF {
				log.Errorf("Error scanning docker log stream: %s", err)
			}
			return
		}
	}
}
