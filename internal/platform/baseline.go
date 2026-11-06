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

// Baseline is the baseline an analysis compares its results with: a LocalBaseline, a CloudBaseline or
// NoBaseline.
type Baseline interface {
	// BaselinePath is the file the analysis results are compared with, empty for NoBaseline.
	BaselinePath() string
	// Source is the qdenv.BaselineSource* value which tells a linter which baseline this CLI has
	// resolved for it, so that the linter reports the source of the baseline and doesn't look for one
	// in Qodana Cloud again.
	Source() string
	// Cleanup removes the baseline this CLI has downloaded for the analysis.
	Cleanup()
	// reportsUsed tells whether this CLI reports which baseline the analysis has used, so that a run
	// reports it once. linterInContainer tells whether the linter runs in a container of its own.
	reportsUsed(linterInContainer bool) bool
	usedMessage() string
}

// ReportUsedBaseline tells which baseline the analysis has used, unless another CLI of the run tells it.
func ReportUsedBaseline(baseline Baseline, linterInContainer bool) {
	if baseline.reportsUsed(linterInContainer) {
		fmt.Println(baseline.usedMessage())
	}
}

// LocalBaseline is a baseline file given to the run.
type LocalBaseline struct {
	path string
	// fromLauncher tells that the CLI which launched this one has given the file
	fromLauncher bool
}

func (b LocalBaseline) BaselinePath() string { return b.path }

func (b LocalBaseline) Source() string { return qdenv.BaselineSourceLocal }

func (b LocalBaseline) Cleanup() {}

// reportsUsed leaves a local baseline of a linter in a container to that linter's CLI, as only it
// knows the path of the file in the container.
func (b LocalBaseline) reportsUsed(linterInContainer bool) bool {
	return b.fromLauncher || !linterInContainer
}

func (b LocalBaseline) usedMessage() string {
	return fmt.Sprintf("The analysis used the baseline file %s", b.path)
}

// CloudBaseline is the baseline stored in Qodana Cloud, downloaded to a file of its own directory.
type CloudBaseline struct {
	path string
	// fromLauncher tells that the CLI which launched this one has downloaded the file, and removes it
	fromLauncher bool
}

func (b CloudBaseline) BaselinePath() string { return b.path }

func (b CloudBaseline) Source() string { return qdenv.BaselineSourceCloud }

func (b CloudBaseline) Cleanup() {
	if !b.fromLauncher {
		removeBaselineDir(filepath.Dir(b.path))
	}
}

// reportsUsed leaves a cloud baseline to the CLI which has downloaded it.
func (b CloudBaseline) reportsUsed(bool) bool { return !b.fromLauncher }

func (b CloudBaseline) usedMessage() string {
	return "The analysis used the baseline from Qodana Cloud"
}

// NoBaseline is a run without a baseline, and the reason for it.
type NoBaseline struct {
	// why tells the user why there is no baseline, empty when the run was given none
	why string
	// advice tells the user how to give the run a baseline, empty when there is nothing to do
	advice string
	// fromLauncher tells that the CLI which launched this one has resolved no baseline, and reports it
	fromLauncher bool
}

func (b NoBaseline) BaselinePath() string { return "" }

func (b NoBaseline) Source() string { return qdenv.BaselineSourceNone }

func (b NoBaseline) Cleanup() {}

func (b NoBaseline) reportsUsed(bool) bool { return !b.fromLauncher }

func (b NoBaseline) usedMessage() string {
	message := "The analysis used no baseline."
	if b.why != "" {
		message += " " + b.why + "."
	}
	if b.advice != "" {
		message += " " + b.advice
	}
	return message
}

// ResolveBaseline returns the baseline the analysis results are compared with: baselineFile, if the
// run was given one, otherwise the baseline stored in Qodana Cloud for the linter of productCode, if
// the project of cloudToken has one. An empty productCode gets the baseline of every tool of the
// project. The downloaded baseline is stored in cacheDir and must be cleaned up.
//
// Qodana Cloud is asked only once: a linter launched by a CLI which has resolved the baseline already
// is told so by the qdenv.QodanaBaselineSource of that CLI, and takes that answer.
func ResolveBaseline(baselineFile string, cloudToken string, productCode string, cacheDir string) Baseline {
	if baseline, given := givenBaseline(baselineFile); given {
		return baseline
	}
	if cloudToken == "" {
		log.Debug("Not connected to Qodana Cloud, running without a baseline")
		return NoBaseline{
			advice: "Give a baseline file with --baseline, or run with the token of a Qodana Cloud project " +
				"which has the cloud baseline turned on.",
		}
	}
	return askCloudBaseline(cloud.GetCloudApiEndpoints().NewBaselineClient(cloudToken), productCode, cacheDir)
}

// givenBaseline returns the baseline given by the command line or by the CLI which launched this one,
// and whether there is one: without it, the baseline is up to Qodana Cloud.
func givenBaseline(baselineFile string) (Baseline, bool) {
	launcherSource := launcherBaselineSource()
	if baselineFile != "" {
		if launcherSource == qdenv.BaselineSourceCloud {
			return CloudBaseline{path: baselineFile, fromLauncher: true}, true
		}
		return LocalBaseline{path: baselineFile, fromLauncher: launcherSource != ""}, true
	}

	if launcherSource == qdenv.BaselineSourceNone {
		log.Debug("The CLI which launched this linter has resolved no baseline for it")
		return NoBaseline{fromLauncher: true}, true
	}
	if launcherSource != "" {
		// a launching CLI passes the file of a baseline it has resolved, so the source has no
		// baseline to back it: it is not trusted to skip Qodana Cloud
		msg.WarningMessage(
			"Ignoring %s=%s, which is set without a baseline file",
			qdenv.QodanaBaselineSource,
			launcherSource,
		)
	}
	return nil, false
}

// askCloudBaseline returns the baseline stored in Qodana Cloud for the linter of productCode, or
// NoBaseline saying why there is none.
func askCloudBaseline(cloudApi *cloud.QdClient, productCode string, cacheDir string) Baseline {
	fmt.Println("Checking for baseline from Qodana Cloud ...")
	toolName := product.CloudBaselineToolName(productCode)
	baseline, err := downloadCloudBaseline(cloudApi, toolName, cacheDir)
	if err != nil {
		msg.WarningMessage("Cannot use the baseline of Qodana Cloud, running without a baseline: %v", err)
		return NoBaseline{why: "The baseline of Qodana Cloud could not be downloaded"}
	}
	if none, ok := baseline.(NoBaseline); ok {
		log.Debugf("Qodana Cloud has no baseline of '%s'", toolName)
		fmt.Printf("%s, running without a baseline\n", none.why)
	}
	return baseline
}

// launcherBaselineSource returns the baseline source which the CLI that launched this one has
// resolved for it. Only a CLI in a Qodana image is launched by another CLI: on a host the variable
// is a leftover of another run.
func launcherBaselineSource() string {
	source := os.Getenv(qdenv.QodanaBaselineSource)
	if source != "" && !qdenv.IsContainer() {
		log.Debugf("Ignoring %s=%s outside of a container", qdenv.QodanaBaselineSource, source)
		return ""
	}
	return source
}

// downloadCloudBaseline stores the baseline from Qodana Cloud as a SARIF file in a directory of its own
// in the cache dir. When there is no cloud baseline to compare the analysis with, it returns NoBaseline
// saying why.
func downloadCloudBaseline(cloudApi *cloud.QdClient, toolName string, cacheDir string) (Baseline, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create the cache dir %s: %w", cacheDir, err)
	}
	dir, err := os.MkdirTemp(cacheDir, "cloud-baseline-")
	if err != nil {
		return nil, fmt.Errorf("failed to create a directory for the baseline: %w", err)
	}
	path := filepath.Join(dir, "qodana.sarif.json")
	status, err := writeCloudBaseline(cloudApi, toolName, path)
	if err == nil && status == cloud.BaselineWritten {
		return CloudBaseline{path: path}, nil
	}
	removeBaselineDir(dir)
	if err != nil {
		return nil, err
	}
	switch status {
	case cloud.BaselineEmpty:
		return NoBaseline{why: "The cloud baseline of the project in Qodana Cloud is empty"}, nil
	case cloud.BaselineDisabled:
		return NoBaseline{
			why:    "The cloud baseline is turned off for this project in Qodana Cloud",
			advice: "Give a baseline file with --baseline, or turn on the cloud baseline.",
		}, nil
	case cloud.BaselineUnsupported:
		return NoBaseline{
			why:    "This Qodana Cloud doesn't serve cloud baselines",
			advice: "Give a baseline file with --baseline.",
		}, nil
	default:
		return nil, fmt.Errorf("unexpected answer of Qodana Cloud to the baseline request: status %d", status)
	}
}

// writeCloudBaseline writes the baseline from Qodana Cloud to the file at path, readable by the linter.
func writeCloudBaseline(cloudApi *cloud.QdClient, toolName string, path string) (cloud.BaselineStatus, error) {
	// a linter in a container reads the baseline as the user of the container, which is not always
	// the user which has downloaded it, while MkdirTemp lets only the owner in
	if err := os.Chmod(filepath.Dir(path), 0o755); err != nil {
		return cloud.BaselineUnknown, fmt.Errorf("failed to open the directory of the baseline: %w", err)
	}
	status, err := cloudApi.WriteBaseline(toolName, path)
	if err != nil || status != cloud.BaselineWritten {
		return status, err
	}
	// os.Create obeys the umask of the host, which can leave it to its owner only
	if err := os.Chmod(path, 0o644); err != nil {
		return cloud.BaselineUnknown, fmt.Errorf("failed to make the baseline readable: %w", err)
	}
	if info, statErr := os.Stat(path); statErr == nil {
		log.Debugf("Baseline of %s from Qodana Cloud: %s, %d bytes", toolName, path, info.Size())
	}
	return status, nil
}

// removeBaselineDir removes the directory a cloud baseline was downloaded to. A directory left behind
// only takes space in the cache dir, so the analysis goes on.
func removeBaselineDir(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		log.Warnf("Failed to remove the downloaded baseline %s: %v", dir, err)
	}
}

// computeBaselinePrintResults runs SARIF analysis (compares with baseline and prints the result)
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
	if ret == 1 {
		return -1, fmt.Errorf("error in supplied arguments for baseline-cli")
	}
	return ret, nil
}
