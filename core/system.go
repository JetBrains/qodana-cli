/*
 * Copyright 2021-2024 JetBrains s.r.o.
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
	"github.com/JetBrains/qodana-cli/v2024/core/corescan"
	"github.com/JetBrains/qodana-cli/v2024/core/startup"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/effectiveconfig"
	"github.com/JetBrains/qodana-cli/v2024/platform/git"
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	"github.com/JetBrains/qodana-cli/v2024/platform/nuget"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	cienvironment "github.com/cucumber/ci-environment/go"
	"github.com/docker/docker/client"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	// DisableCheckUpdates flag to disable checking for updates
	DisableCheckUpdates = false

	releaseUrl = "https://api.github.com/repos/JetBrains/qodana-cli/releases/latest"
)

// CheckForUpdates check GitHub https://github.com/JetBrains/qodana-cli/ for the latest version of CLI release.
func CheckForUpdates(currentVersion string) {
	if currentVersion == "dev" || strings.HasSuffix(
		currentVersion,
		"nightly",
	) || qdenv.IsContainer() || cienvironment.DetectCIEnvironment() != nil || DisableCheckUpdates {
		return
	}
	latestVersion := getLatestVersion()
	if latestVersion != "" && latestVersion != currentVersion {
		msg.WarningMessage(
			"New version of %s CLI is available: %s. See https://jb.gg/qodana-cli/update\n",
			msg.PrimaryBold("qodana"),
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
	return strings.TrimPrefix(result["tag_name"].(string), "v")
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
func RunAnalysis(ctx context.Context, c corescan.Context) int {
	log.Debug("Running analysis with options")
	platform.LogContext(&c)

	if !utils.IsInstalled("git") && (c.FullHistory() || c.Commit() != "" || c.DiffStart() != "" || c.DiffEnd() != "") {
		log.Fatal("Cannot use git related functionality without a git executable")
	}

	startHash, err := c.StartHash()
	if err != nil {
		log.Fatal(err)
	}

	scenario := c.DetermineRunScenario(startHash != "")
	if scenario != corescan.RunScenarioDefault && !git.RevisionExists(c.ProjectDir(), startHash, c.LogDir()) {
		msg.WarningMessageCI(
			"Cannot run analysis for commit %s because it doesn't exist in the repository. Check that you retrieve the full git history before running Qodana.",
			startHash,
		)
		scenario = corescan.RunScenarioDefault

		// backoff to regular analysis
		c = c.BackoffToDefaultAnalysisBecauseOfMissingCommit()
	}

	installPlugins(c)
	// this way of running needs to do bootstrap twice on different commits and will do it internally
	if scenario != corescan.RunScenarioScoped && c.Ide() != "" {
		utils.Bootstrap(c.QodanaYamlConfig().Bootstrap, c.ProjectDir())
	}
	switch scenario {
	case corescan.RunScenarioFullHistory:
		return runWithFullHistory(ctx, c, startHash)
	case corescan.RunScenarioLocalChanges:
		return runLocalChanges(ctx, c, startHash)
	case corescan.RunScenarioScoped:
		return runScopeScript(ctx, c, startHash)
	case corescan.RunScenarioDefault:
		return runQodana(ctx, c)
	default:
		log.Fatalf("Unknown run scenario %s", scenario)
		panic("Unreachable")
	}
}

func runLocalChanges(ctx context.Context, c corescan.Context, startHash string) int {
	var exitCode int
	gitReset := false
	r, err := git.CurrentRevision(c.ProjectDir(), c.LogDir())
	if err != nil {
		log.Fatal(err)
	}
	if c.DiffEnd() != "" && c.DiffEnd() != r {
		msg.WarningMessage("Cannot run local-changes because --diff-end is %s and HEAD is %s", c.DiffEnd(), r)
	} else {
		err := git.Reset(c.ProjectDir(), startHash, c.LogDir())
		if err != nil {
			msg.WarningMessage("Could not reset git repository, no --commit option will be applied: %s", err)
		} else {
			c = c.ForcedLocalChanges()
			gitReset = true
		}
	}

	exitCode = runQodana(ctx, c)

	if gitReset {
		_ = git.ResetBack(c.ProjectDir(), c.LogDir())
	}
	return exitCode
}

func runWithFullHistory(ctx context.Context, c corescan.Context, startHash string) int {
	remoteUrl, err := git.RemoteUrl(c.ProjectDir(), c.LogDir())
	if err != nil {
		log.Fatal(err)
	}
	branch, err := git.Branch(c.ProjectDir(), c.LogDir())
	if err != nil {
		log.Fatal(err)
	}
	if remoteUrl == "" && branch == "" {
		log.Fatal("Please check that project is located within the Git repo")
	}

	err = git.Clean(c.ProjectDir(), c.LogDir())
	if err != nil {
		log.Fatal(err)
	}
	revisions := git.Revisions(c.ProjectDir())
	allCommits := len(revisions)
	counter := 0
	var exitCode int

	if startHash != "" {
		for i, revision := range revisions {
			counter++
			if revision == startHash {
				revisions = revisions[i:]
				break
			}
		}
	}

	for _, revision := range revisions {
		counter++

		msg.WarningMessage("[%d/%d] Running analysis for revision %s", counter+1, allCommits, revision)
		err = git.CheckoutAndUpdateSubmodule(c.ProjectDir(), revision, true, c.LogDir())
		if err != nil {
			log.Fatal(err)
		}
		msg.EmptyMessage()

		contextForAnalysis := c.WithVcsEnvForFullHistoryAnalysisIteration(remoteUrl, branch, revision)
		exitCode = runQodana(ctx, contextForAnalysis)
	}
	err = git.CheckoutAndUpdateSubmodule(c.ProjectDir(), branch, true, c.LogDir())
	if err != nil {
		log.Fatal(err)
	}
	return exitCode
}

func runScopeScript(ctx context.Context, c corescan.Context, startHash string) int {
	// don't run this logic when we're about to launch a container - it's just double work
	if c.Ide() == "" {
		return runQodana(ctx, c)
	}
	var err error
	end := c.DiffEnd()
	if end == "" {
		end, err = git.CurrentRevision(c.ProjectDir(), c.LogDir())
		if err != nil {
			log.Fatal(err)
		}
	}

	scopeFile, err := writeChangesFile(c, startHash, end)
	if err != nil {
		log.Fatal("Failed to prepare diff run ", err)
	}
	defer func() {
		_ = os.Remove(scopeFile)
	}()

	runFunc := func(hash string, c corescan.Context) (bool, int) {
		e := git.CheckoutAndUpdateSubmodule(c.ProjectDir(), hash, true, c.LogDir())
		if e != nil {
			log.Fatalf("Cannot checkout commit %s: %v", hash, e)
		}

		startup.PrepareDirectories(c.Prod(), c.CacheDir(), c.LogDir(), c.ConfigDir())
		log.Infof("Analysing %s", hash)

		// for CLI, we use only bootstrap from this effective yaml
		// all other fields are used from the one (effective aswell) obtained at the start
		localQodanaYamlFullPath := qdyaml.GetLocalNotEffectiveQodanaYamlFullPath(
			c.ProjectDir(),
			c.CustomLocalQodanaYamlPath(),
		)
		effectiveConfigFiles, err := effectiveconfig.CreateEffectiveConfigFiles(
			localQodanaYamlFullPath,
			c.GlobalConfigurationsFile(),
			c.GlobalConfigurationId(),
			c.Prod().JbrJava(),
			c.QodanaSystemDir(),
			"qdconfig-"+hash,
			c.LogDir(),
		)
		if err != nil {
			log.Fatalf("Failed to load Qodana configuration during analysis of commit %s: %v", hash, err)
		}

		// if local qodana yaml doesn't exist on revision, for bootstrap fallback to the one constructed at the start
		var bootstrap string
		if c.LocalQodanaYamlExists() {
			yaml := qdyaml.LoadQodanaYamlByFullPath(effectiveConfigFiles.EffectiveQodanaYamlPath)
			bootstrap = yaml.Bootstrap
		} else {
			bootstrap = c.QodanaYamlConfig().Bootstrap
		}
		utils.Bootstrap(bootstrap, c.ProjectDir())

		contextForAnalysis := c.WithEffectiveConfigurationDirOnRevision(effectiveConfigFiles.ConfigDir)
		exitCode := runQodana(ctx, contextForAnalysis) // TODO WHY qodana yaml is not passed further to runQodana?
		if !(exitCode == 0 || exitCode == 255) {
			log.Errorf("Qodana analysis on %s exited with code %d. Aborting", hash, exitCode)
			return true, exitCode
		}
		return false, exitCode
	}

	startRunContext := c.FirstStageOfScopedScript(scopeFile)
	stop, code := runFunc(startHash, startRunContext)
	if stop {
		return code
	}

	startSarif := platform.GetSarifPath(startRunContext.ResultsDir())

	endRunContext := c.SecondStageOfScopedScript(scopeFile, startSarif)
	stop, code = runFunc(end, endRunContext)
	if stop {
		return code
	}

	err = utils.CopyDir(endRunContext.ResultsDir(), c.ResultsDir())
	if err != nil {
		log.Fatal(err)
	}

	saveReport(c)
	return code
}

// writeChangesFile creates a temp file containing the changes between diffStart and diffEnd
func writeChangesFile(c corescan.Context, start string, end string) (string, error) {
	if start == "" || end == "" {
		return "", fmt.Errorf("no commits given")
	}
	changedFiles, err := git.ComputeChangedFiles(c.ProjectDir(), start, end, c.LogDir())
	if err != nil {
		return "", err
	}

	if len(changedFiles.Files) == 0 {
		return "", fmt.Errorf("nothing to compare between %s and %s", start, end)
	}
	file, err := os.CreateTemp("", "diff-scope.txt")
	if err != nil {
		return "", err
	}
	defer func() {
		err := file.Close()
		if err != nil {
			log.Warn("Failed to close scope file ", err)
		}
	}()

	jsonChanges, err := json.MarshalIndent(changedFiles, "", "  ")
	if err != nil {
		return "", err
	}
	_, err = file.WriteString(string(jsonChanges))
	if err != nil {
		return "", fmt.Errorf("failed to write scope file: %w", err)
	}

	err = utils.CopyFile(file.Name(), filepath.Join(c.LogDir(), "changes.json"))
	if err != nil {
		return "", err
	}

	return file.Name(), nil
}

func runQodana(ctx context.Context, c corescan.Context) int {
	var exitCode int
	var err error
	if c.Linter() != "" {
		exitCode = runQodanaContainer(ctx, c)
	} else if c.Ide() != "" {
		nuget.UnsetNugetVariables() // TODO: get rid of it from 241 release
		exitCode, err = runQodanaLocal(c)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatal("No linter or IDE specified")
	}

	return exitCode
}

// followLinter follows the linter logs and prints the progress.
func followLinter(client *client.Client, containerName string, progress *pterm.SpinnerPrinter, scanStages []string) {
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
	interactive := msg.IsInteractive()
	for scanner.Scan() {
		line := scanner.Text()
		if !interactive && len(line) >= dockerSpecialCharsLength {
			line = line[dockerSpecialCharsLength:]
		}

		line = strings.TrimSuffix(line, "\n")
		if err == nil || len(line) > 0 {
			if strings.Contains(line, "Starting up") {
				msg.UpdateText(progress, scanStages[2])
			}
			if strings.Contains(line, "The Project opening stage completed in") {
				msg.UpdateText(progress, scanStages[3])
			}
			if strings.Contains(line, "The Project configuration stage completed in") {
				msg.UpdateText(progress, scanStages[4])
			}
			if strings.Contains(line, "Detailed summary") {
				msg.UpdateText(progress, scanStages[5])
				if !msg.IsInteractive() {
					msg.EmptyMessage()
				}
			}
			msg.PrintLinterLog(line)
		}
		if err != nil {
			if err != io.EOF {
				log.Errorf("Error scanning docker log stream: %s", err)
			}
			return
		}
	}
}

func getScanStages() []string {
	scanStages := []string{
		"Preparing Qodana Docker images",
		"Starting the analysis engine",
		"Opening the project",
		"Configuring the project",
		"Analyzing the project",
		"Preparing the report",
	}
	for i, stage := range scanStages {
		scanStages[i] = msg.PrimaryBold("[%d/%d] ", i+1, len(scanStages)+1) + msg.Primary(stage)
	}
	return scanStages
}

// saveReport saves web files to expect, and generates json.
func saveReport(c corescan.Context) {
	prod := c.Prod()
	if !(qdenv.IsContainer() && (c.SaveReport() || c.ShowReport())) {
		return
	}

	reportConverter := filepath.Join(prod.IdeBin(), "intellij-report-converter.jar")
	if _, err := os.Stat(reportConverter); os.IsNotExist(err) {
		log.Fatal("Not able to save the report: report-converter is missing")
		return
	}
	log.Println("Generating HTML report ...")
	if res, err := utils.RunCmd(
		"",
		utils.QuoteForWindows(prod.JbrJava()),
		"-jar",
		utils.QuoteForWindows(reportConverter),
		"-s",
		utils.QuoteForWindows(c.ProjectDir()),
		"-d",
		utils.QuoteForWindows(c.ResultsDir()),
		"-o",
		utils.QuoteForWindows(platform.ReportResultsPath(c.ReportDir())),
		"-n",
		"result-allProblems.json",
		"-f",
	); res > 0 || err != nil {
		os.Exit(res)
	}
	err := utils.CopyDir(filepath.Join(prod.Home, "web"), c.ReportDir())
	if err != nil {
		log.Fatal("Not able to save the report: ", err)
		return
	}
}
