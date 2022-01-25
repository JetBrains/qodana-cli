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
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
)

type QodanaOptions struct { // TODO: get available options from the image / have another scheme
	ResultsDir            string
	CacheDir              string
	ProjectDir            string
	Linter                string
	SourceDirectory       string
	DisableSanity         bool
	ProfileName           string
	ProfilePath           string
	RunPromo              bool
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
	Token                 string
	AnalysisId            string
	EnvVariables          []string
	UnveilProblems        bool
}

var (
	Version     = "dev"
	DoNotTrack  = false
	Interrupted = false
	scanStages  = []string{
		"Preparing Qodana Docker images",
		"Starting the analysis engine",
		"Opening the project",
		"Configuring the project",
		"Analyzing the project",
		"Preparing the report",
	}
)

// Contains checks if a string is in a given slice.
func Contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// CheckLinter validates the image used for the scan.
func CheckLinter(image string) {
	if !strings.HasPrefix(image, OfficialDockerPrefix) {
		WarningMessage("You are using an unofficial Qodana linter: " + image + "\n")
		UnofficialLinter = true
	}
	for _, linter := range notSupportedLinters {
		if linter == image {
			log.Fatalf("%s is not supported by Qodana CLI", linter)
		}
	}
}

// GetLinterSystemDir returns path to <userCacheDir>/JetBrains/<linter>/<project-hash>/
func GetLinterSystemDir(project string, linter string) string {
	userCacheDir, _ := os.UserCacheDir()
	linterDirName := strings.Replace(strings.Replace(linter, ":", "-", -1), "/", "-", -1)
	projectAbs, _ := filepath.Abs(project)

	return filepath.Join(
		userCacheDir,
		"JetBrains",
		linterDirName,
		fmt.Sprintf("%x", sha256.Sum256([]byte(projectAbs))),
	)
}

// PrepareFolders cleans up report folder, creates the necessary folders for the analysis
func PrepareFolders(opts *QodanaOptions) {
	linterHome := GetLinterSystemDir(opts.ProjectDir, opts.Linter)
	if opts.ResultsDir == "" {
		opts.ResultsDir = filepath.Join(linterHome, "results")
	}
	if opts.CacheDir == "" {
		opts.CacheDir = filepath.Join(linterHome, "cache")
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
		fmt.Sprintf("Showing Qodana report at http://localhost:%d, press Ctrl+C to stop", port),
		"",
	)
}

// openReport serves the report on the given port and opens the browser.
func openReport(path string, port int) {
	url := fmt.Sprintf("http://localhost:%d", port)
	go func() {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			err := openBrowser(url)
			if err != nil {
				return
			}
		}
	}()
	http.Handle("/", http.FileServer(http.Dir(path)))
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		WarningMessage(fmt.Sprintf("Problem serving report, %s\n", err.Error()))
		return
	}
	_, _ = fmt.Scan()
}

// openBrowser opens the default browser to the given url
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// OpenDir opens directory in the default file manager
func OpenDir(path string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "explorer"
		args = []string{"/select"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, path)
	return exec.Command(cmd, args...).Start()
}

// RunLinter runs the linter with the given options.
func RunLinter(ctx context.Context, options *QodanaOptions) int {
	docker, err := client.NewClientWithOpts()
	if err != nil {
		log.Fatal("couldn't instantiate docker client", err)
	}
	for i, stage := range scanStages {
		scanStages[i] = PrimaryBold.Sprintf("[%d/%d] ", i+1, len(scanStages)+1) + Primary.Sprint(stage)
	}
	CheckLinter(options.Linter)
	var progress *pterm.SpinnerPrinter
	if IsInteractive() {
		progress, _ = StartQodanaSpinner(scanStages[0])
	}

	pullImage(ctx, docker, options.Linter)
	dockerOpts := getDockerOptions(options)
	updateText(progress, scanStages[1])
	runContainer(ctx, docker, dockerOpts)

	reader, _ := docker.ContainerLogs(context.Background(), dockerOpts.Name, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
	})
	defer func(reader io.ReadCloser) {
		err := reader.Close()
		if err != nil {
			log.Fatal(err.Error())
		}
	}(reader)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Starting up") {
			updateText(progress, scanStages[2])
		}
		if strings.Contains(line, "The Project opening stage completed in") {
			updateText(progress, scanStages[3])
		}
		if strings.Contains(line, "The Project configuration stage completed in") {
			updateText(progress, scanStages[4])
		}
		if strings.Contains(line, "---- Qodana - Detailed summary ----") {
			updateText(progress, scanStages[5])
			if !IsInteractive() {
				pterm.Println()
			}
		}
		if strings.Contains(line, "IDEA exit code:") {
			break
		}
		printLinterLog(line)
	}
	exitCode := getDockerExitCode(ctx, docker, dockerOpts.Name)
	if progress != nil {
		_ = progress.Stop()
	}
	return int(exitCode)
}
