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

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JetBrains/qodana-cli/internal/cloud"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	"github.com/JetBrains/qodana-cli/internal/tooling"
	log "github.com/sirupsen/logrus"
)

// baselineDownloader writes the baseline of a tool from Qodana Cloud to a file.
type baselineDownloader interface {
	WriteBaseline(toolName string, file *os.File) (bool, error)
}

var noBaselineCleanup = func() {}

// Baseline is the baseline an analysis compares its results with.
type Baseline struct {
	baselinePath             string
	isCloudBaseline          bool
	removeDownloadedBaseline func()
}

func (b Baseline) BaselinePath() string { return b.baselinePath }

func (b Baseline) IsFromCloud() bool { return b.isCloudBaseline }

func (b Baseline) Cleanup() {
	if b.removeDownloadedBaseline != nil {
		b.removeDownloadedBaseline()
	}
}

// UsedMessage tells which baseline the analysis has used.
func (b Baseline) UsedMessage() string {
	switch {
	case b.isCloudBaseline:
		return "The analysis used the baseline from Qodana Cloud"
	case b.baselinePath != "":
		return fmt.Sprintf("The analysis used the baseline file %s", b.baselinePath)
	default:
		return "The analysis used no baseline. Give a baseline file with --baseline, " +
			"or turn on the cloud baseline of the project in Qodana Cloud."
	}
}

// ResolveBaseline returns the baseline the analysis results are compared with: baselineFile, if the
// run was given one, otherwise the baseline stored in Qodana Cloud for toolName, if the project of
// cloudToken has one. An empty toolName gets the baseline of every tool of the project.
// The downloaded baseline is stored in cacheDir and must be cleaned up.
func ResolveBaseline(baselineFile string, cloudToken string, toolName string, cacheDir string) (Baseline, error) {
	if baselineFile != "" {
		return Baseline{baselinePath: baselineFile, removeDownloadedBaseline: noBaselineCleanup}, nil
	}

	noBaseline := Baseline{removeDownloadedBaseline: noBaselineCleanup}
	if cloudToken == "" {
		log.Debug("Not connected to Qodana Cloud, running without a baseline")
		return noBaseline, nil
	}
	// the baseline of a linter is the one of the tool its report is written under, e.g. QDNET. A
	// custom image, of which the CLI knows no product code, gets the baseline of every tool of the
	// project: the problems of the other tools are simply never matched.
	if toolName == "" {
		log.Debug("No product code of the linter, getting the baseline of all tools of the project")
	}

	fmt.Println("Fetching baseline from Qodana Cloud ...")
	client := cloud.GetCloudApiEndpoints().NewLintersApiClient(cloudToken)
	baseline, cleanup, err := downloadCloudBaseline(client, toolName, cacheDir)
	if err != nil {
		return noBaseline, fmt.Errorf("failed to get the baseline from Qodana Cloud: %w", err)
	}
	if baseline == "" {
		log.Debugf("Qodana Cloud has no baseline of '%s' for this project", toolName)
		return noBaseline, nil
	}
	return Baseline{baselinePath: baseline, isCloudBaseline: true, removeDownloadedBaseline: cleanup}, nil
}

// downloadCloudBaseline stores the baseline from Qodana Cloud as a SARIF file in the cache dir.
// An empty path means that there is no cloud baseline to compare the analysis with.
func downloadCloudBaseline(client baselineDownloader, toolName string, cacheDir string) (string, func(), error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", noBaselineCleanup, fmt.Errorf("failed to create the cache dir %s: %w", cacheDir, err)
	}
	dir, err := os.MkdirTemp(cacheDir, "cloud-baseline-")
	if err != nil {
		return "", noBaselineCleanup, fmt.Errorf("failed to create a directory for the baseline: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	// a linter in a container reads the baseline as the user of the container, which is not always
	// the user which has downloaded it, while MkdirTemp lets only the owner in
	if err := os.Chmod(dir, 0o755); err != nil {
		cleanup()
		return "", noBaselineCleanup, fmt.Errorf("failed to open the directory of the baseline: %w", err)
	}
	baseline := filepath.Join(dir, "qodana.sarif.json")
	file, err := os.Create(baseline)
	if err != nil {
		cleanup()
		return "", noBaselineCleanup, fmt.Errorf("failed to create the baseline file %s: %w", baseline, err)
	}

	written, err := client.WriteBaseline(toolName, file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil || !written {
		cleanup()
		return "", noBaselineCleanup, err
	}
	if info, statErr := os.Stat(baseline); statErr == nil {
		log.Debugf("Baseline of %s from Qodana Cloud: %s, %d bytes", toolName, baseline, info.Size())
	}
	return baseline, cleanup, nil
}

// computeBaselinePrintResults runs SARIF analysis (compares with baseline and prints the result)=
func computeBaselinePrintResults(c thirdpartyscan.Context, thresholds map[string]string, baseline string) (int, error) {
	sarifPath := GetSarifPath(c.ResultsDir())
	args := []string{
		tooling.GetQodanaJBRPath(c.CacheDir()),
		// baseline-cli -> Clikt -> Mordant -> JNA
		// https://ajalt.github.io/mordant/guide/#__tabbed_1_2
		"--enable-native-access=ALL-UNNAMED",
		"-jar",
		tooling.BaselineCli.GetLibPath(c.CacheDir()),
		"-r",
		sarifPath,
	}
	severities := thresholdsToArgs(thresholds)
	args = append(args, severities...)
	if baseline != "" {
		args = append(args, "-b", baseline)
	}
	if c.BaselineIncludeAbsent() {
		args = append(args, "-i")
	}
	_, _, ret, err := utils.LaunchAndLog(c.LogDir(), "baseline", args)
	if err != nil {
		return -1, fmt.Errorf("error while running baseline-cli: %w", err)
	}
	if ret > 0 {
		if ret == 1 {
			return -1, fmt.Errorf("error in supplied arguments for baseline-cli")
		}
		return ret, nil
	}
	return ret, nil
}
