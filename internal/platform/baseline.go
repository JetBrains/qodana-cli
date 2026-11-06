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
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	"github.com/JetBrains/qodana-cli/internal/tooling"
	log "github.com/sirupsen/logrus"
)

// baselineDownloader writes the baseline of a tool from Qodana Cloud to a file.
type baselineDownloader interface {
	WriteBaseline(toolName string, file *os.File) (cloud.BaselineStatus, error)
}

var noBaselineCleanup = func() {}

// Baseline is the baseline an analysis compares its results with.
type Baseline struct {
	baselinePath             string
	source                   string
	cloudStatus              cloud.BaselineStatus
	removeDownloadedBaseline func()
}

func (b Baseline) BaselinePath() string { return b.baselinePath }

func (b Baseline) IsFromCloud() bool { return b.source == qdenv.BaselineSourceCloud }

// Source tells a linter which baseline this CLI has resolved for it, so that the linter reports the
// source of the baseline and doesn't look for one in Qodana Cloud again.
func (b Baseline) Source() string { return b.source }

func (b Baseline) Cleanup() {
	if b.removeDownloadedBaseline != nil {
		b.removeDownloadedBaseline()
	}
}

// UsedMessage tells which baseline the analysis has used.
func (b Baseline) UsedMessage() string {
	switch {
	case b.source == qdenv.BaselineSourceCloud:
		return "The analysis used the baseline from Qodana Cloud"
	case b.baselinePath != "":
		return fmt.Sprintf("The analysis used the baseline file %s", b.baselinePath)
	case b.cloudStatus == cloud.BaselineEmpty:
		return "The analysis used no baseline: the cloud baseline of the project in Qodana Cloud is empty."
	case b.cloudStatus == cloud.BaselineUnsupported:
		return "The analysis used no baseline. Give a baseline file with --baseline: " +
			"this Qodana Cloud doesn't serve cloud baselines."
	default:
		return "The analysis used no baseline. Give a baseline file with --baseline, " +
			"or turn on the cloud baseline of the project in Qodana Cloud."
	}
}

// ResolveBaseline returns the baseline the analysis results are compared with: baselineFile, if the
// run was given one, otherwise the baseline stored in Qodana Cloud for toolName, if the project of
// cloudToken has one. An empty toolName gets the baseline of every tool of the project.
// The downloaded baseline is stored in cacheDir and must be cleaned up.
//
// Qodana Cloud is asked only once: a linter launched by a CLI which has resolved the baseline
// already is told so by launcherSource, the qdenv.QodanaBaselineSource of that CLI, and takes that
// answer. A CLI which nothing has launched passes no launcherSource.
func ResolveBaseline(
	baselineFile string,
	launcherSource string,
	cloudToken string,
	toolName string,
	cacheDir string,
) Baseline {
	newClient := func() baselineDownloader {
		return cloud.GetCloudApiEndpoints().NewLintersApiClient(cloudToken)
	}
	return resolveBaseline(baselineFile, launcherSource, cloudToken != "", toolName, cacheDir, newClient)
}

func resolveBaseline(
	baselineFile string,
	launcherSource string,
	connected bool,
	toolName string,
	cacheDir string,
	newClient func() baselineDownloader,
) Baseline {
	if baselineFile != "" {
		source := launcherSource
		if source != qdenv.BaselineSourceCloud {
			source = qdenv.BaselineSourceLocal
		}
		return Baseline{
			baselinePath:             baselineFile,
			source:                   source,
			removeDownloadedBaseline: noBaselineCleanup,
		}
	}

	noBaseline := Baseline{source: qdenv.BaselineSourceNone, removeDownloadedBaseline: noBaselineCleanup}
	switch launcherSource {
	case "":
	case qdenv.BaselineSourceNone:
		log.Debug("The CLI which launched this linter has resolved no baseline for it")
		return noBaseline
	default:
		// a launching CLI passes the file of a baseline it has resolved, so the source has no
		// baseline to back it: it is not trusted to skip Qodana Cloud
		msg.WarningMessage(
			"Ignoring %s=%s, which is set without a baseline file",
			qdenv.QodanaBaselineSource,
			launcherSource,
		)
	}
	if !connected {
		log.Debug("Not connected to Qodana Cloud, running without a baseline")
		return noBaseline
	}
	fmt.Println("Checking for baseline from Qodana Cloud ...")
	toolName = product.CloudBaselineToolName(toolName)
	baseline, status, cleanup, err := downloadCloudBaseline(newClient(), toolName, cacheDir)
	if err != nil {
		msg.WarningMessage("Cannot use the baseline of Qodana Cloud, running without a baseline: %v", err)
		return noBaseline
	}
	noBaseline.cloudStatus = status
	switch status {
	case cloud.BaselineWritten:
		return Baseline{
			baselinePath:             baseline,
			source:                   qdenv.BaselineSourceCloud,
			cloudStatus:              status,
			removeDownloadedBaseline: cleanup,
		}
	case cloud.BaselineEmpty:
		fmt.Printf(
			"The cloud baseline of %s in Qodana Cloud is empty, running without a baseline\n",
			baselineOwner(toolName),
		)
	case cloud.BaselineDisabled:
		fmt.Println("The cloud baseline is turned off for this project in Qodana Cloud, running without a baseline")
	case cloud.BaselineUnsupported:
		fmt.Println("This Qodana Cloud doesn't serve cloud baselines, running without a baseline")
	}
	return noBaseline
}

// baselineOwner names whose cloud baseline was asked for: no tool name asks for the whole project.
func baselineOwner(toolName string) string {
	if toolName == "" {
		return "the project"
	}
	return "'" + toolName + "'"
}

// downloadCloudBaseline stores the baseline from Qodana Cloud as a SARIF file in the cache dir.
// The path is given only for a cloud.BaselineWritten status: any other one tells why there is no
// cloud baseline to compare the analysis with.
func downloadCloudBaseline(
	client baselineDownloader,
	toolName string,
	cacheDir string,
) (string, cloud.BaselineStatus, func(), error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", 0, noBaselineCleanup, fmt.Errorf("failed to create the cache dir %s: %w", cacheDir, err)
	}
	dir, err := os.MkdirTemp(cacheDir, "cloud-baseline-")
	if err != nil {
		return "", 0, noBaselineCleanup, fmt.Errorf("failed to create a directory for the baseline: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	// a linter in a container reads the baseline as the user of the container, which is not always
	// the user which has downloaded it, while MkdirTemp lets only the owner in
	if err := os.Chmod(dir, 0o755); err != nil {
		cleanup()
		return "", 0, noBaselineCleanup, fmt.Errorf("failed to open the directory of the baseline: %w", err)
	}
	baseline := filepath.Join(dir, "qodana.sarif.json")
	file, err := os.Create(baseline)
	if err != nil {
		cleanup()
		return "", 0, noBaselineCleanup, fmt.Errorf("failed to create the baseline file %s: %w", baseline, err)
	}

	status, err := client.WriteBaseline(toolName, file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		cleanup()
		return "", 0, noBaselineCleanup, err
	}
	if status != cloud.BaselineWritten {
		cleanup()
		return "", status, noBaselineCleanup, nil
	}
	// os.Create obeys the umask of the host, which can leave the file to its owner only
	if err := os.Chmod(baseline, 0o644); err != nil {
		cleanup()
		return "", 0, noBaselineCleanup, fmt.Errorf("failed to make the baseline readable: %w", err)
	}
	if info, statErr := os.Stat(baseline); statErr == nil {
		log.Debugf("Baseline of %s from Qodana Cloud: %s, %d bytes", toolName, baseline, info.Size())
	}
	return baseline, status, cleanup, nil
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
