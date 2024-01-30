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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/docker/docker/client"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	cienvironment "github.com/cucumber/ci-environment/go"

	"github.com/pterm/pterm"

	log "github.com/sirupsen/logrus"
)

var (
	// DisableCheckUpdates flag to disable checking for updates
	DisableCheckUpdates = false

	scanStages []string
	releaseUrl = "https://api.github.com/repos/JetBrains/qodana-cli/releases/latest"
)

// CheckForUpdates check GitHub https://github.com/JetBrains/qodana-cli/ for the latest version of CLI release.
func CheckForUpdates(currentVersion string) {
	if currentVersion == "dev" || platform.IsContainer() || cienvironment.DetectCIEnvironment() != nil || DisableCheckUpdates {
		return
	}
	latestVersion := getLatestVersion()
	if latestVersion != "" && latestVersion != currentVersion {
		platform.WarningMessage(
			"New version of %s CLI is available: %s. See https://jb.gg/qodana-cli/update\n",
			platform.PrimaryBold("qodana"),
			latestVersion,
		)
		DisableCheckUpdates = true
	}
}

// getLatestVersion returns the latest published version of the CLI.
func getLatestVersion() string {
	resp, err := http.Get(releaseUrl)
	if err != nil {
		return ""
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			return
		}
	}(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return ""
	}
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	result := make(map[string]interface{})
	err = json.Unmarshal(bodyText, &result)
	if err != nil {
		return ""
	}
	return result["tag_name"].(string)
}

// OpenDir opens directory in the default file manager
func OpenDir(path string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "explorer"
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, path)
	return exec.Command(cmd, args...).Start()
}

// prepareHost gets the current user, creates the necessary folders for the analysis.
func prepareHost(opts *QodanaOptions) {
	if opts.ClearCache {
		err := os.RemoveAll(opts.CacheDir)
		if err != nil {
			log.Errorf("Could not clear local Qodana cache: %s", err)
		}
	}
	platform.WarnIfPrivateFeedDetected(Prod.Code, opts.ProjectDir)
	if platform.IsNugetConfigNeeded() {
		platform.PrepareNugetConfig(os.Getenv("HOME"))
	}
	if err := os.MkdirAll(opts.CacheDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	if err := os.MkdirAll(opts.ResultsDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	if opts.Linter != "" {
		PrepareContainerEnvSettings()
	}
	if opts.Ide != "" {
		if platform.Contains(platform.AllNativeCodes, strings.TrimSuffix(opts.Ide, EapSuffix)) {
			platform.PrintProcess(func(spinner *pterm.SpinnerPrinter) {
				if spinner != nil {
					spinner.ShowTimer = false // We will update interactive spinner
				}
				opts.Ide = downloadAndInstallIDE(opts, opts.GetQodanaSystemDir(), spinner)
			}, fmt.Sprintf("Downloading %s", opts.Ide), fmt.Sprintf("downloading IDE distribution to %s", opts.GetQodanaSystemDir()))
		} else {
			val, exists := os.LookupEnv(platform.QodanaDistEnv)
			if !exists || val == "" || opts.Ide != val {
				log.Fatalf("Product code %s is not supported", opts.Ide)
			}
		}
		prepareLocalIdeSettings(opts)
	}
	if opts.RequiresToken(Prod.IsCommunity() || Prod.EAP) {
		opts.ValidateToken(false)
	}
}

// IsHomeDirectory returns true if the given path is the user's home directory.
func IsHomeDirectory(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return absPath == home
}

// RunAnalysis runs the linter with the given options.
func RunAnalysis(ctx context.Context, options *QodanaOptions) int {
	log.Debugf("Running analysis with options: %+v", options)
	prepareHost(options)

	var exitCode int

	if options.FullHistory && isInstalled("git") {
		remoteUrl := platform.GitRemoteUrl(options.ProjectDir)
		branch := platform.GitBranch(options.ProjectDir)
		if remoteUrl == "" && branch == "" {
			log.Fatal("Please check that project is located within the Git repo")
		}
		options.Setenv(platform.QodanaRemoteUrl, remoteUrl)
		options.Setenv(platform.QodanaBranch, branch)

		err := platform.GitClean(options.ProjectDir)
		if err != nil {
			log.Fatal(err)
		}
		revisions := platform.GitRevisions(options.ProjectDir)
		allCommits := len(revisions)
		counter := 0
		if options.Commit != "" {
			for i, revision := range revisions {
				counter++
				if revision == options.Commit {
					revisions = revisions[i:]
					break
				}
			}
		}

		for _, revision := range revisions {
			counter++
			options.Setenv(platform.QodanaRevision, revision)
			platform.WarningMessage("[%d/%d] Running analysis for revision %s", counter+1, allCommits, revision)
			err = platform.GitCheckout(options.ProjectDir, revision)
			if err != nil {
				log.Fatal(err)
			}
			platform.EmptyMessage()

			exitCode = runQodana(ctx, options)
			options.Unsetenv(platform.QodanaRevision)
		}
		err = platform.GitCheckout(options.ProjectDir, branch)
		if err != nil {
			log.Fatal(err)
		}
	} else if options.Commit != "" && isInstalled("git") {
		options.GitReset = false
		err := platform.GitReset(options.ProjectDir, options.Commit)
		if err != nil {
			platform.WarningMessage("Could not reset git repository, no --commit option will be applied: %s", err)
		} else {
			options.GitReset = true
		}

		exitCode = runQodana(ctx, options)

		if options.GitReset && !strings.HasPrefix(options.Commit, "CI") {
			_ = platform.GitResetBack(options.ProjectDir)
		}
	} else {
		exitCode = runQodana(ctx, options)
	}

	return exitCode
}

func runQodana(ctx context.Context, options *QodanaOptions) int {
	var exitCode int
	var err error
	if options.Linter != "" {
		exitCode = runQodanaContainer(ctx, options)
	} else if options.Ide != "" {
		platform.UnsetNugetVariables() // TODO: get rid of it from 241 release
		exitCode, err = runQodanaLocal(options)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatal("No linter or IDE specified")
	}

	return exitCode
}

// followLinter follows the linter logs and prints the progress.
func followLinter(client *client.Client, containerName string, progress *pterm.SpinnerPrinter) {
	reader, err := client.ContainerLogs(context.Background(), containerName, containerLogsOptions)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer func(reader io.ReadCloser) {
		err := reader.Close()
		if err != nil {
			log.Fatal(err.Error())
		}
	}(reader)
	scanner := bufio.NewScanner(reader)
	interactive := platform.IsInteractive()
	for scanner.Scan() {
		line := scanner.Text()
		if !interactive && len(line) >= dockerSpecialCharsLength {
			line = line[dockerSpecialCharsLength:]
		}

		line = strings.TrimSuffix(line, "\n")
		if err == nil || len(line) > 0 {
			if strings.Contains(line, "Starting up") {
				platform.UpdateText(progress, scanStages[2])
			}
			if strings.Contains(line, "The Project opening stage completed in") {
				platform.UpdateText(progress, scanStages[3])
			}
			if strings.Contains(line, "The Project configuration stage completed in") {
				platform.UpdateText(progress, scanStages[4])
			}
			if strings.Contains(line, "Detailed summary") {
				platform.UpdateText(progress, scanStages[5])
				if !platform.IsInteractive() {
					platform.EmptyMessage()
				}
			}
			platform.PrintLinterLog(line)
		}
		if err != nil {
			if err != io.EOF {
				log.Errorf("Error scanning docker log stream: %s", err)
			}
			return
		}
	}
}

func resetScanStages() {
	scanStages = []string{
		"Preparing Qodana Docker images",
		"Starting the analysis engine",
		"Opening the project",
		"Configuring the project",
		"Analyzing the project",
		"Preparing the report",
	}
}

const (
	qodanaAppInfoFilename = "QodanaAppInfo.xml"
	m2                    = ".m2"
	nuget                 = "nuget"
)

// saveReport saves web files to expect, and generates json.
func saveReport(opts *QodanaOptions) {
	if platform.IsContainer() {
		reportConverter := filepath.Join(Prod.IdeBin(), "intellij-report-converter.jar")
		if _, err := os.Stat(reportConverter); os.IsNotExist(err) {
			log.Fatal("Not able to save the report: report-converter is missing")
			return
		}
		log.Println("Generating HTML report ...")
		if res, err := platform.RunCmd("", platform.QuoteForWindows(Prod.JbrJava()), "-jar", platform.QuoteForWindows(reportConverter), "-s", platform.QuoteForWindows(opts.ProjectDir), "-d", platform.QuoteForWindows(opts.ResultsDir), "-o", platform.QuoteForWindows(opts.ReportResultsPath()), "-n", "result-allProblems.json", "-f"); res > 0 || err != nil {
			os.Exit(res)
		}
		if res, err := platform.RunCmd("", "sh", "-c", fmt.Sprintf("cp -r %s/web/* ", Prod.Home)+opts.ReportDir); res > 0 || err != nil {
			os.Exit(res)
		}
	}
}
