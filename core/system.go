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
	"github.com/JetBrains/qodana-cli/v2024/platform"
	cienvironment "github.com/cucumber/ci-environment/go"
	"github.com/docker/docker/client"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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
	if currentVersion == "dev" || strings.HasSuffix(currentVersion, "nightly") || platform.IsContainer() || cienvironment.DetectCIEnvironment() != nil || DisableCheckUpdates {
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

// prepareHost gets the current user, creates the necessary folders for the analysis.
func prepareHost(opts *QodanaOptions) {
	if opts.ClearCache {
		err := os.RemoveAll(opts.CacheDir)
		if err != nil {
			log.Errorf("Could not clear local Qodana cache: %s", err)
		}
	}
	platform.WarnIfPrivateFeedDetected(opts.Linter, opts.ProjectDir)
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
				fixWindowsPlugins(opts.Ide)
			}, fmt.Sprintf("Downloading %s", opts.Ide), fmt.Sprintf("downloading IDE distribution to %s", opts.GetQodanaSystemDir()))
		} else {
			val, exists := os.LookupEnv(platform.QodanaDistEnv)
			if !exists || val == "" || opts.Ide != val { // very strange check
				log.Fatalf("Product code %s is not supported", opts.Ide)
			}
		}
		prepareLocalIdeSettings(opts)
	}
	if opts.RequiresToken(Prod.IsCommunity() || Prod.EAP) {
		opts.ValidateToken(false)
	}
}

// fixWindowsPlugins quick-fix for Windows 241 distributions
func fixWindowsPlugins(ideDir string) {
	if runtime.GOOS == "windows" && strings.Contains(ideDir, "241") {
		pluginsClasspath := filepath.Join(ideDir, "plugins", "plugin-classpath.txt")
		if _, err := os.Stat(pluginsClasspath); err == nil {
			err = os.Remove(pluginsClasspath)
			if err != nil {
				log.Warnf("Failed to remove plugin-classpath.txt: %v", err)
			}
		}
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
	log.Debug("Running analysis with options")
	options.LogOptions()
	prepareHost(options)

	if !isInstalled("git") && (options.FullHistory || options.Commit != "" || options.DiffStart != "" || options.DiffEnd != "") {
		log.Fatal("Cannot use git related functionality without a git executable")
	}

	if strings.HasPrefix(options.Commit, "CI") {
		options.Commit = strings.TrimPrefix(options.Commit, "CI")
	}
	startHash, err := options.StartHash()
	if err != nil {
		log.Fatal(err)
	}

	scenario := options.determineRunScenario(startHash != "")
	if scenario != runScenarioDefault && !platform.GitRevisionExists(options.ProjectDir, startHash, options.LogDirPath()) {
		platform.WarningMessageCI("Cannot run analysis for commit %s because it doesn't exist in the repository. Check that you retrieve the full git history before running Qodana.", startHash)
		scenario = runScenarioDefault
		options.ResetScanScenarioOptions()
	}
	// this way of running needs to do bootstrap twice on different commits and will do it internally
	if scenario != runScenarioScoped && options.Ide != "" {
		platform.Bootstrap(options.QdConfig.Bootstrap, options.ProjectDir)
		installPlugins(options.QdConfig.Plugins)
	}
	switch scenario {
	case runScenarioFullHistory:
		return runWithFullHistory(ctx, options, startHash)
	case runScenarioLocalChanges:
		return runLocalChanges(ctx, options, startHash)
	case runScenarioScoped:
		return runScopeScript(ctx, options, startHash)
	case runScenarioDefault:
		return runQodana(ctx, options)
	default:
		log.Fatalf("Unknown run scenario %s", scenario)
		panic("Unreachable")
	}
}

func runLocalChanges(ctx context.Context, options *QodanaOptions, startHash string) int {
	var exitCode int
	gitReset := false
	r, err := platform.GitCurrentRevision(options.ProjectDir, options.LogDirPath())
	if err != nil {
		log.Fatal(err)
	}
	if options.DiffEnd != "" && options.DiffEnd != r {
		platform.WarningMessage("Cannot run local-changes because --diff-end is %s and HEAD is %s", options.DiffEnd, r)
	} else {
		err := platform.GitReset(options.ProjectDir, startHash, options.LogDirPath())
		if err != nil {
			platform.WarningMessage("Could not reset git repository, no --commit option will be applied: %s", err)
		} else {
			options.Script = "local-changes"
			gitReset = true
		}
	}

	exitCode = runQodana(ctx, options)

	if gitReset {
		_ = platform.GitResetBack(options.ProjectDir, options.LogDirPath())
	}
	return exitCode
}

func runWithFullHistory(ctx context.Context, options *QodanaOptions, startHash string) int {
	remoteUrl, err := platform.GitRemoteUrl(options.ProjectDir, options.LogDirPath())
	if err != nil {
		log.Fatal(err)
	}
	branch, err := platform.GitBranch(options.ProjectDir, options.LogDirPath())
	if err != nil {
		log.Fatal(err)
	}
	if remoteUrl == "" && branch == "" {
		log.Fatal("Please check that project is located within the Git repo")
	}
	options.Setenv(platform.QodanaRemoteUrl, remoteUrl)
	options.Setenv(platform.QodanaBranch, branch)

	err = platform.GitClean(options.ProjectDir, options.LogDirPath())
	if err != nil {
		log.Fatal(err)
	}
	revisions := platform.GitRevisions(options.ProjectDir)
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
		options.Setenv(platform.QodanaRevision, revision)
		platform.WarningMessage("[%d/%d] Running analysis for revision %s", counter+1, allCommits, revision)
		err = platform.GitCheckout(options.ProjectDir, revision, true, options.LogDirPath())
		if err != nil {
			log.Fatal(err)
		}
		platform.EmptyMessage()

		exitCode = runQodana(ctx, options)
		options.Unsetenv(platform.QodanaRevision)
	}
	err = platform.GitCheckout(options.ProjectDir, branch, true, options.LogDirPath())
	if err != nil {
		log.Fatal(err)
	}
	return exitCode
}

func runScopeScript(ctx context.Context, options *QodanaOptions, startHash string) int {
	// don't run this logic when we're about to launch a container - it's just double work
	if options.Ide == "" {
		return runQodana(ctx, options)
	}
	var err error
	end := options.DiffEnd
	if end == "" {
		end, err = platform.GitCurrentRevision(options.ProjectDir, options.LogDirPath())
		if err != nil {
			log.Fatal(err)
		}
	}

	scopeFile, err := writeChangesFile(options, startHash, end)
	if err != nil {
		log.Fatal("Failed to prepare diff run ", err)
	}
	defer func() {
		_ = os.Remove(scopeFile)
	}()

	fixesStrategy := options.FixesStrategy
	applyFixes := options.ApplyFixes
	cleanup := options.Cleanup
	resultsDir := options.ResultsDir
	showReport := options.ShowReport
	saveReportOpt := options.SaveReport
	props := options.Property
	baseline := options.Baseline

	runFunc := func(hash string) (bool, int) {
		e := platform.GitCheckout(options.ProjectDir, hash, true, options.LogDirPath())
		if e != nil {
			log.Fatalf("Cannot checkout commit %s: %v", hash, e)
		}

		prepareDirectories(
			options.CacheDir,
			options.LogDirPath(),
			options.ConfDirPath(),
		)
		log.Infof("Analysing %s", hash)
		writeProperties(options)

		configAtHash, e := platform.GetQodanaYaml(options.ProjectDir)
		if e != nil {
			log.Warnf("Could not read qodana yaml at %s: %v. Using last known config", hash, e)
			configAtHash = options.QdConfig
		}
		platform.Bootstrap(configAtHash.Bootstrap, options.ProjectDir)
		installPlugins(configAtHash.Plugins)

		exitCode := runQodana(ctx, options)
		if !(exitCode == 0 || exitCode == 255) {
			log.Errorf("Qodana analysis on %s exited with code %d. Aborting", hash, exitCode)
			return true, exitCode
		}
		return false, exitCode
	}

	options.Script = platform.QuoteForWindows("scoped:" + scopeFile)
	options.ShowReport = false
	options.SaveReport = false

	startDir := filepath.Join(resultsDir, "start")
	options.Property = append(
		options.Property,
		"-Dqodana.skip.result=true",               // don't print results
		"-Dqodana.skip.coverage.computation=true") // don't compute coverage on first pass
	options.Baseline = ""
	options.ResultsDir = startDir
	options.ApplyFixes = false
	options.Cleanup = false
	options.FixesStrategy = "none" // this option is deprecated, but the only way to overwrite the possible yaml value

	stop, code := runFunc(startHash)
	if stop {
		return code
	}

	startSarif := options.GetSarifPath()

	endDir := filepath.Join(resultsDir, "end")
	options.Property = append(
		props,
		"-Dqodana.skip.preamble=true", // don't print the QD logo again
		"-Didea.headless.enable.statistics=false",                   // disable statistics for second run
		fmt.Sprintf("-Dqodana.scoped.baseline.path=%s", startSarif), // disable statistics for second run
		"-Dqodana.skip.coverage.issues.reporting=true",              // don't report coverage issues on the second pass, but allow numbers to be computed
	)
	options.Baseline = baseline
	options.ResultsDir = endDir
	options.ApplyFixes = applyFixes
	options.Cleanup = cleanup
	options.FixesStrategy = fixesStrategy

	stop, code = runFunc(end)
	if stop {
		return code
	}

	err = platform.CopyDir(options.ResultsDir, resultsDir)
	if err != nil {
		log.Fatal(err)
	}
	options.ResultsDir = resultsDir
	options.ShowReport = showReport
	options.SaveReport = saveReportOpt

	saveReport(options)
	return code
}

// writeChangesFile creates a temp file containing the changes between diffStart and diffEnd
func writeChangesFile(options *QodanaOptions, start string, end string) (string, error) {
	if start == "" || end == "" {
		return "", fmt.Errorf("no commits given")
	}
	changedFiles, err := platform.GitChangedFiles(options.ProjectDir, start, end, options.LogDirPath())
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

	err = platform.CopyFile(file.Name(), filepath.Join(options.LogDirPath(), "changes.json"))
	if err != nil {
		return "", err
	}

	return file.Name(), nil
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
	m2    = ".m2"
	nuget = "nuget"
)

// saveReport saves web files to expect, and generates json.
func saveReport(opts *QodanaOptions) {
	if !(platform.IsContainer() && (opts.SaveReport || opts.ShowReport)) {
		return
	}

	reportConverter := filepath.Join(Prod.IdeBin(), "intellij-report-converter.jar")
	if _, err := os.Stat(reportConverter); os.IsNotExist(err) {
		log.Fatal("Not able to save the report: report-converter is missing")
		return
	}
	log.Println("Generating HTML report ...")
	if res, err := platform.RunCmd("", platform.QuoteForWindows(Prod.JbrJava()), "-jar", platform.QuoteForWindows(reportConverter), "-s", platform.QuoteForWindows(opts.ProjectDir), "-d", platform.QuoteForWindows(opts.ResultsDir), "-o", platform.QuoteForWindows(opts.ReportResultsPath()), "-n", "result-allProblems.json", "-f"); res > 0 || err != nil {
		os.Exit(res)
	}
	err := platform.CopyDir(filepath.Join(Prod.Home, "web"), opts.ReportDir)
	if err != nil {
		log.Fatal("Not able to save the report: ", err)
		return
	}
}
