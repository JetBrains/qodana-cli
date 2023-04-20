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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/docker/docker/client"

	cienvironment "github.com/cucumber/ci-environment/go"

	"github.com/pterm/pterm"

	log "github.com/sirupsen/logrus"
)

var (
	unofficialLinter = false
	// DisableCheckUpdates flag to disable checking for updates
	DisableCheckUpdates = false

	scanStages = []string{
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

// CheckForUpdates check GitHub https://github.com/JetBrains/qodana-cli/ for the latest version of CLI release.
func CheckForUpdates(currentVersion string) {
	if currentVersion == "dev" || cienvironment.DetectCIEnvironment() != nil || DisableCheckUpdates {
		return
	}
	latestVersion := getLatestVersion()
	if latestVersion != "" && latestVersion != currentVersion {
		WarningMessage(
			"New version of %s CLI is available: %s. See https://jb.gg/qodana-cli/update\n",
			PrimaryBold("qodana"),
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

// openReport serves the report on the given port and opens the browser.
func openReport(cloudUrl string, path string, port int) {
	if cloudUrl != "" {
		resp, err := http.Get(cloudUrl)
		if err == nil && resp.StatusCode == 200 {
			err = openBrowser(cloudUrl)
			if err != nil {
				return
			}
		}
	} else {
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
		http.Handle("/", noCache(http.FileServer(http.Dir(path))))
		err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
		if err != nil {
			WarningMessage("Problem serving report, %s\n", err.Error())
			return
		}
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
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, path)
	return exec.Command(cmd, args...).Start()
}

// noCache handles serving the static files with no cache headers.
func noCache(h http.Handler) http.Handler {
	etagHeaders := []string{
		"ETag",
		"If-Modified-Since",
		"If-Match",
		"If-None-Match",
		"If-Range",
		"If-Unmodified-Since",
	}
	epoch := time.Unix(0, 0).Format(time.RFC1123)
	noCacheHeaders := map[string]string{
		"Expires":         epoch,
		"Cache-Control":   "no-cache, private, max-age=0",
		"Pragma":          "no-cache",
		"X-Accel-Expires": "0",
	}
	fn := func(w http.ResponseWriter, r *http.Request) {
		for _, x := range etagHeaders {
			if r.Header.Get(x) != "" {
				r.Header.Del(x)
			}
		}
		for k, v := range noCacheHeaders {
			w.Header().Set(k, v)
		}
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// getId returns the project/linter id for internal CLI usage from the given path.
func getId(project string) string {
	projectAbs, _ := filepath.Abs(project)
	sha256sum := sha256.Sum256([]byte(projectAbs))
	return hex.EncodeToString(sha256sum[:])[0:8]
}

// GetLinterSystemDir returns path to <userCacheDir>/JetBrains/<linter>/<project-id>/.
func GetLinterSystemDir(project string, linter string) string {
	userCacheDir, _ := os.UserCacheDir()
	return filepath.Join(
		userCacheDir,
		"JetBrains",
		"Qodana",
		getId(linter),
		getId(project),
	)
}

// checkLinter validates the image used for the scan.
func checkLinter(image string) {
	if !strings.HasPrefix(image, officialImagePrefix) {
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
	if opts.ClearCache {
		err := os.RemoveAll(opts.CacheDir)
		if err != nil {
			log.Errorf("Could not clear local Qodana cache: %s", err)
		}
	}
	if opts.User == "" {
		switch runtime.GOOS {
		case "windows":
			opts.User = "root"
		default: // "darwin", "linux", "freebsd", "openbsd", "netbsd"
			opts.User = fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
		}
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

// AskUserConfirm asks the user for confirmation with yes/no.
func AskUserConfirm(what string) bool {
	if !IsInteractive() {
		return false
	}
	prompt := qodanaInteractiveConfirm
	prompt.DefaultText = "\n?  " + what
	answer, err := prompt.Show()
	if err != nil {
		log.Fatalf("Error while waiting for user input: %s", err)
	}
	return answer
}

// RunLinter runs the linter with the given options.
func RunLinter(ctx context.Context, options *QodanaOptions) int {
	options.GitReset = false
	if options.Commit != "" && isGitInstalled() {
		err := gitReset(options.ProjectDir, options.Commit)
		if err != nil {
			WarningMessage("Could not reset git repository, no --commit option will be applied: %s", err)
		} else {
			options.GitReset = true
		}
	}
	docker := getContainerClient()
	for i, stage := range scanStages {
		scanStages[i] = PrimaryBold("[%d/%d] ", i+1, len(scanStages)+1) + primary(stage)
	}
	checkLinter(options.Linter)
	if unofficialLinter {
		WarningMessage("You are using an unofficial Qodana linter: %s\n", options.Linter)
	}
	progress, _ := startQodanaSpinner(scanStages[0])
	if !(options.SkipPull) {
		PullImage(docker, options.Linter)
	}
	dockerConfig := getDockerOptions(options)
	updateText(progress, scanStages[1])
	runContainer(ctx, docker, dockerConfig)
	go followLinter(docker, dockerConfig.Name, progress, options.ResultsDir)
	exitCode := getContainerExitCode(ctx, docker, dockerConfig.Name)
	if options.GitReset && !strings.HasPrefix(options.Commit, "CI") {
		_ = gitResetBack(options.ProjectDir)
	}
	if progress != nil {
		_ = progress.Stop()
	}
	return int(exitCode)
}

// followLinter follows the linter logs and prints the progress.
func followLinter(client *client.Client, containerName string, progress *pterm.SpinnerPrinter, resultsDir string) {
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
	interactive := IsInteractive()
	for scanner.Scan() {
		line := scanner.Text()
		if !interactive && len(line) >= dockerSpecialCharsLength {
			line = line[dockerSpecialCharsLength:]
		}

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
			if strings.Contains(line, "Report is successfully uploaded to ") {
				reportUrl := strings.TrimPrefix(line, "Report is successfully uploaded to ")
				saveReportUrl(resultsDir, reportUrl)
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

// GetReportUrl get Qodana Cloud report URL from the given qodana.sarif.json
func GetReportUrl(resultsDir string) string {
	filePath := filepath.Join(resultsDir, QodanaReportUrlFile)
	if _, err := os.Stat(filePath); err == nil {
		url, err := os.ReadFile(filePath)
		if err != nil {
			log.Fatal(err)
		}
		return string(url)
	}
	return ""
}

// saveReportUrl saves the report URL to the resultsDir/qodana.cloud file.
func saveReportUrl(resultsDir, reportUrl string) {
	if reportUrl == "" {
		return
	}
	resultsDir = filepath.Join(resultsDir, "qodana.cloud")
	err := os.WriteFile(resultsDir, []byte(reportUrl), 0o644)
	if err != nil {
		log.Errorf("Could not save the report URL to the results directory: %s", err)
	}
}
