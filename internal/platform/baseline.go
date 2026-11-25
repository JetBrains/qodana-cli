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
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/JetBrains/qodana-cli/internal/cloud"
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/telemetry"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	"github.com/JetBrains/qodana-cli/internal/tooling"
	log "github.com/sirupsen/logrus"
)

// baselineDownloader writes the baseline of a tool from Qodana Cloud to the file at path.
type baselineDownloader interface {
	WriteBaseline(toolName string, path string) (cloud.BaselineStatus, error)
}

// Baseline is the baseline an analysis compares its results with: a LocalBaseline, a CloudBaseline or
// NoBaseline. QodanaBaseline of the linter has the same kinds.
type Baseline interface {
	// BaselinePath is the file the analysis results are compared with, empty for NoBaseline.
	BaselinePath() string
	// Cleanup removes the baseline this CLI has downloaded for the analysis.
	Cleanup()
	// UsedMessage tells which baseline the analysis has used, with the words of the linter.
	UsedMessage() string
}

// LocalBaseline is a baseline file given to the run.
type LocalBaseline struct {
	path string
}

func (b LocalBaseline) BaselinePath() string { return b.path }

func (b LocalBaseline) Cleanup() {}

func (b LocalBaseline) UsedMessage() string {
	return fmt.Sprintf("The analysis used the baseline file %s", b.path)
}

// CloudBaseline is the baseline stored in Qodana Cloud, downloaded to a file in a directory of its own.
type CloudBaseline struct {
	path string
}

func (b CloudBaseline) BaselinePath() string { return b.path }

func (b CloudBaseline) Cleanup() { removeBaselineDir(filepath.Dir(b.path)) }

func (b CloudBaseline) UsedMessage() string {
	return "The analysis used the baseline from Qodana Cloud"
}

// NoBaseline is a run without a baseline.
type NoBaseline struct {
	// reason tells why the run has no baseline and what to do. Without one, the message gives the
	// advice of the linter.
	reason string
}

func (b NoBaseline) BaselinePath() string { return "" }

func (b NoBaseline) Cleanup() {}

func (b NoBaseline) UsedMessage() string {
	if b.reason == "" {
		return "The analysis used no baseline. Give a baseline file with --baseline, " +
			"or turn on the cloud baseline of the project in Qodana Cloud."
	}
	return "The analysis used no baseline: " + b.reason
}

// ResolveBaseline returns the baseline the analysis results are compared with: baselineFile, if the
// run was given one, otherwise the baseline stored in Qodana Cloud for toolName, if the project of
// cloudToken has one. An empty toolName gets the baseline of every tool of the project.
// The downloaded baseline is stored in cacheDir and must be cleaned up. A relative baselineFile is of projectDir or
// of the current dir.
//
// Call it after the analysis, just before the comparison, so that a failed analysis downloads
// nothing. An IDE linter compares its results itself, so it downloads its cloud baseline itself.
func ResolveBaseline(
	baselineFile string,
	projectDir string,
	cloudToken string,
	toolName string,
	cacheDir string,
) Baseline {
	newClient := func() baselineDownloader {
		return cloud.GetCloudApiEndpoints().NewBaselineClient(cloudToken)
	}
	return resolveBaseline(baselineFile, projectDir, cloudToken != "", toolName, cacheDir, newClient)
}

// LinterBaselineResolved records the baseline the CLI gives an IDE linter: the baseline file of the run, or none
// without a connection to Qodana Cloud. The linter resolves the cloud baseline itself, after the analysis, so the
// CLI records nothing for it: see telemetry.ProblemsFound.
func LinterBaselineResolved(baselineFile string, projectDir string, connected bool) {
	switch {
	case baselineFile != "":
		localBaselineResolved(baselineFile, projectDir)
	case !connected:
		telemetry.BaselineResolved("none", "not_connected", 0, 0)
	}
}

func localBaselineResolved(baselineFile string, projectDir string) {
	status := "missing"
	if fileExists(baselineFile) || !filepath.IsAbs(baselineFile) && fileExists(filepath.Join(projectDir, baselineFile)) {
		status = "found"
	}
	telemetry.BaselineResolved("local", status, 0, 0)
}

func fileExists(file string) bool {
	_, err := os.Stat(file)
	return err == nil
}

func resolveBaseline(
	baselineFile string,
	projectDir string,
	connected bool,
	toolName string,
	cacheDir string,
	newClient func() baselineDownloader,
) Baseline {
	if baselineFile != "" {
		localBaselineResolved(baselineFile, projectDir)
		return LocalBaseline{path: baselineFile}
	}
	if !connected {
		telemetry.BaselineResolved("none", "not_connected", 0, 0)
		log.Debug("Not connected to Qodana Cloud, running without a baseline")
		return NoBaseline{}
	}
	fmt.Println("Checking for baseline from Qodana Cloud ...")
	toolName = product.CloudBaselineToolName(toolName)
	start := time.Now()
	baseline, status, err := downloadCloudBaseline(newClient(), toolName, cacheDir)
	if err != nil {
		failure, httpStatus := cloudBaselineFailure(err)
		telemetry.BaselineResolved("cloud", failure, httpStatus, time.Since(start))
		msg.WarningMessage("Cannot use the baseline of Qodana Cloud, running without a baseline: %v", err)
		return NoBaseline{reason: "the baseline of Qodana Cloud could not be downloaded."}
	}
	telemetry.BaselineResolved("cloud", status, 0, time.Since(start))
	return baseline
}

// cloudBaselineFailure tells why the baseline of Qodana Cloud is not downloaded, with the HTTP status of the answer.
func cloudBaselineFailure(err error) (string, int) {
	var netError net.Error
	var apiError *cloud.APIError
	switch {
	case errors.As(err, new(*fs.PathError)):
		return "io_error", 0
	// a read which waits too long fails the download
	case errors.As(err, &netError) && netError.Timeout(), errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, context.Canceled):
		return "timeout", 0
	case errors.Is(err, cloud.ErrInvalidBaseline), errors.Is(err, io.ErrUnexpectedEOF):
		return "invalid_response", 0
	case errors.As(err, &apiError):
		return "request_failed", apiError.StatusCode
	default:
		return "request_failed", 0
	}
}

// downloadCloudBaseline stores the baseline from Qodana Cloud as a SARIF file in a directory of its
// own in the cache dir. When Qodana Cloud has no baseline to compare the analysis with, it returns
// NoBaseline, which tells why. The status is the baseline_status of the statistics.
func downloadCloudBaseline(client baselineDownloader, toolName string, cacheDir string) (Baseline, string, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, "", fmt.Errorf("failed to create the cache dir %s: %w", cacheDir, err)
	}
	dir, err := os.MkdirTemp(cacheDir, "cloud-baseline-")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create a directory for the baseline: %w", err)
	}
	path := filepath.Join(dir, "qodana.sarif.json")
	status, err := client.WriteBaseline(toolName, path)
	if err == nil && status == cloud.BaselineWritten {
		if info, statErr := os.Stat(path); statErr == nil {
			log.Debugf("Baseline of %s from Qodana Cloud: %s, %d bytes", toolName, path, info.Size())
		}
		return CloudBaseline{path: path}, "found", nil
	}
	removeBaselineDir(dir)
	if err != nil {
		return nil, "", err
	}
	switch status {
	case cloud.BaselineEmpty:
		log.Debugf("The cloud baseline of '%s' is empty", toolName)
		fmt.Println("The cloud baseline of the project in Qodana Cloud is empty, running without a baseline")
		return NoBaseline{reason: "the cloud baseline of the project in Qodana Cloud is empty."}, "empty", nil
	case cloud.BaselineDisabled:
		fmt.Println("The cloud baseline is turned off for this project in Qodana Cloud, running without a baseline")
		return NoBaseline{}, "disabled", nil
	case cloud.BaselineUnsupported:
		fmt.Println("This Qodana Cloud doesn't serve cloud baselines, running without a baseline")
		return NoBaseline{
			reason: "this Qodana Cloud doesn't serve cloud baselines. Give a baseline file with --baseline.",
		}, "unsupported", nil
	default:
		return nil, "", fmt.Errorf("unexpected answer of Qodana Cloud to the baseline request: status %d", status)
	}
}

// removeBaselineDir removes the directory a cloud baseline was downloaded to. A directory left behind
// only takes space in the cache dir, so the analysis goes on.
func removeBaselineDir(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		log.Warnf("Failed to remove the downloaded baseline %s: %v", dir, err)
	}
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
