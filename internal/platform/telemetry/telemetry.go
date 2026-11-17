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

// Package telemetry sends anonymous Qodana CLI usage statistics to JetBrains FUS and to PostHog.
//
// Events are kept in memory with the time they happened and sent by Finish, so nothing leaves the machine
// when sending turns out to be disallowed later in the run. Nothing is collected and JetBrains is not even
// contacted when the user opted out or the run has a license-only token.
package telemetry

import (
	"cmp"
	"context"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JetBrains/qodana-cli/internal/core/exitcodes"
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	cienvironment "github.com/cucumber/ci-environment/go"
	log "github.com/sirupsen/logrus"
)

const (
	doNotTrackEnv = "DO_NOT_TRACK"
	// senderDeadline bounds, from the start of the run, how long Finish waits for the FUS config, which is fetched
	// when it is not cached: the runs failing early are the ones the funnel is about
	senderDeadline = 3 * time.Second
)

type event struct {
	Group string         `json:"group"`
	Id    string         `json:"id"`
	Time  int64          `json:"time"`
	Data  map[string]any `json:"data,omitempty"`
}

var (
	// disabled turns statistics off in tests of other packages; tests of this package turn them on
	disabled = testing.Testing()
	// connect creates the sender, tests replace it
	connect = newSender

	mu        sync.Mutex
	enabled   bool
	allowed   = true
	command   string
	projectId string
	orgId     string
	scenario  string
	startedAt time.Time
	analyzing bool
	// stage is the last funnel step the run reached, failedStage the step a container run failed at
	stage       string
	failedStage string
	tokenSource string
	// baselineFound tells that the analysis is given a baseline
	baselineFound bool
	// knownReason is the precise reason of a failure, which its exit code does not tell
	knownReason string
	events      []event
	senders     chan *sender
	collected   chan struct{}
	finishOnce  sync.Once
)

// OptedOut reports whether the user turned statistics off, for the CLI and for the linters it runs.
func OptedOut(noStatistics bool) bool {
	return noStatistics || isTruthy(os.Getenv(doNotTrackEnv))
}

// Start begins collecting statistics for the command; nothing is collected when the user opted out. deviceId gives
// the device id the CLI gives the linters it runs, it is computed only when statistics are on.
func Start(cmd string, deviceId func() string, noStatistics bool) {
	if disabled || OptedOut(noStatistics) || isLicenseOnly() {
		return
	}
	now := time.Now()
	// the CLI that started this container reports the run: nothing is sent from here
	inContainer := handoverFile() != ""
	var id string
	var ready chan *sender
	var done chan struct{}
	if !inContainer {
		// deviceId runs git: not under mu
		id, ready, done = deviceId(), make(chan *sender, 1), make(chan struct{})
	}
	mu.Lock()
	enabled, command, startedAt, senders, collected = true, cmd, now, ready, done
	mu.Unlock()
	log.RegisterExitHandler(func() { Finish(1, ReasonFatal) })

	if !inContainer {
		// the sender fetches the FUS config over the network: prepare it while the command runs
		go func() {
			var s *sender
			defer func() { ready <- s }()
			defer recoverPanic()
			s = connect(id)
		}()
		go func() {
			defer close(done)
			defer recoverPanic()
			system, software := environment()
			track(groupEnv, "system", system)
			track(groupEnv, "software", software)
		}()
	}

	track(
		groupRun,
		started,
		map[string]any{"ci": ciName(), "container": qdenv.IsContainer(), "interactive": msg.IsInteractive()},
	)
}

// ciName names the CI the CLI runs on: ci-environment knows neither TeamCity nor, as qdenv does, Space and Bitbucket.
func ciName() string {
	switch {
	case os.Getenv("TEAMCITY_VERSION") != "":
		return "teamcity"
	case os.Getenv("JB_SPACE_API_URL") != "":
		return "space"
	case qdenv.IsBitBucket():
		return "bitbucket"
	case os.Getenv("CODEBUILD_BUILD_ID") != "":
		return "aws-codebuild"
	}
	if env := cienvironment.DetectCIEnvironment(); env != nil {
		return oneOf(qdenv.GetCIName(env), ciNames)
	}
	// most CIs set CI, also the ones not detected above: none is a run outside CI
	if ci := os.Getenv("CI"); ci != "" && ci != "false" {
		return other
	}
	return "none"
}

func recoverPanic() {
	if r := recover(); r != nil {
		log.Debugf("Failed to prepare statistics: %v", r)
	}
}

// isLicenseOnly reports whether the run uses a license-only token, which forbids sending statistics. It is known
// from the environment before the license is checked, also on the host of a linter container which never checks it.
func isLicenseOnly() bool {
	return os.Getenv(qdenv.QodanaLicenseOnlyToken) != "" && os.Getenv(qdenv.QodanaToken) == ""
}

// Allow permits sending the statistics, it is forbidden for license-only tokens.
func Allow(isAllowed bool) {
	mu.Lock()
	defer mu.Unlock()
	allowed = allowed && isAllowed
}

// SetCloudIds sets the Qodana Cloud project and organization id hashes attached to every run event.
func SetCloudIds(projectIdHash string, organizationIdHash string) {
	mu.Lock()
	defer mu.Unlock()
	projectId, orgId = projectIdHash, organizationIdHash
}

func LinterReady(linter string, version string, paid bool, eap bool, mode string) {
	// eap tells an EAP build
	version, _, _ = strings.Cut(version, "-")
	if version != "latest" {
		version = versionName(version)
	}
	track(
		groupRun,
		linterReady,
		map[string]any{"linter": linter, "version": version, "paid": paid, "eap": eap, "mode": mode},
	)
}

func LicenseObtained(kind string, plan string) {
	data := map[string]any{"kind": kind}
	if plan != "" {
		data["plan"] = oneOf(plan, globalRules.Enums[enumLicenseType])
	}
	track(groupRun, licenseObtained, data)
}

// SetTokenSource sets where the Qodana Cloud token comes from: the environment, an organization token exchanged
// for it, the keychain or the user prompt. The first source is the one of the token.
func SetTokenSource(source string) {
	mu.Lock()
	defer mu.Unlock()
	tokenSource = cmp.Or(tokenSource, source)
}

// RejectLicense records the rejection as the reason of the failure the caller exits with right after: log.Fatal
// prints its message and finishes the run, an exit without the logrus exit handlers must call Finish.
func RejectLicense(reason string) {
	mu.Lock()
	defer mu.Unlock()
	record(groupRun, licenseRejected, map[string]any{"reject_reason": reason})
	knownReason = ReasonLicenseRejected
}

// BaselineResolved records which baseline the analysis compares its results with, a local file or the one of
// Qodana Cloud, and its status: found, or why the analysis runs without it. httpStatus is the answer of a failed
// Qodana Cloud request.
func BaselineResolved(source string, status string, httpStatus int, downloadTime time.Duration) {
	data := map[string]any{"source": source, "baseline_status": status}
	if httpStatus != 0 {
		data["http_status"] = httpStatus
	}
	if downloadTime != 0 {
		data["request_ms"] = downloadTime.Milliseconds()
	}
	mu.Lock()
	defer mu.Unlock()
	// a missing baseline file is no baseline: problems.found reports baseline=none
	baselineFound = status == "found"
	record(groupRun, baselineResolved, data)
}

// ReportPublished records whether the report reached Qodana Cloud; it is not published without an upload token.
func ReportPublished(hasToken bool, uploaded bool) {
	status := "skipped"
	if uploaded {
		status = "uploaded"
	} else if hasToken {
		status = "failed"
	}
	track(groupRun, reportPublished, map[string]any{"publish_status": status})
}

// LinterSelected records the linter qodana init configures the project with, or that the project has one already.
func LinterSelected(linter string, preconfigured bool) {
	data := map[string]any{"preconfigured": preconfigured}
	if linter != "" {
		data["linter"] = linter
	}
	track(groupRun, linterSelected, data)
}

// LinterDownloaded records the download of the linter or the pull of its image, cached if the linter or the image
// is up to date already.
func LinterDownloaded(mode string, cached bool, duration time.Duration) {
	trackOnce(
		groupRun, linterDownloaded, map[string]any{
			"mode": mode, "cached": cached, "download_ms": duration.Milliseconds(),
		},
	)
}

// DockerReady records the container engine the linter runs in: its limits, not the host ones, apply to the linter.
func DockerReady(cpuCount int, memoryBytes int64, engineVersion string) {
	// a distribution build like 20.10.24+dfsg1 or 26.1.4-ce is reported by its version
	if i := strings.IndexAny(engineVersion, "+-"); i >= 0 {
		engineVersion = engineVersion[:i]
	}
	trackOnce(
		groupEnv, dockerEngine, map[string]any{
			"cpu_count": powerOfTwo(uint64(cpuCount), cpuCounts),
			"ram_gb":    powerOfTwo(uint64(memoryBytes/gb), memorySizes),
			"version":   versionName(engineVersion),
		},
	)
}

// ProblemsFound records the numbers of problems of the report by their SARIF baseline states, in buckets, and
// whether the analysis applied the baseline it was given: only then the problems have baseline states. Absent
// problems are of the baseline, not of the project.
func ProblemsFound(countsByBaselineState map[string]int) {
	total := -countsByBaselineState["absent"]
	for _, count := range countsByBaselineState {
		total += count
	}
	data := map[string]any{
		"total":     bucket(total),
		"new":       bucket(countsByBaselineState["new"]),
		"unchanged": bucket(countsByBaselineState["unchanged"]),
		"absent":    bucket(countsByBaselineState["absent"]),
	}
	mu.Lock()
	defer mu.Unlock()
	switch {
	case !baselineFound:
		data["baseline"] = "none"
	case total+countsByBaselineState["absent"] == 0:
		// a report without problems does not tell
		data["baseline"] = "unknown"
	case countsByBaselineState[""] == 0:
		data["baseline"] = "applied"
	default:
		data["baseline"] = "ignored"
	}
	record(groupRun, problemsFound, data)
}

// bucket gives the order of magnitude of a count: 0, 1+, 10+, 100+ and so on.
func bucket(count int) string {
	if count == 0 {
		return "0"
	}
	magnitude := 1
	for count >= magnitude*10 && magnitude < 100000 {
		magnitude *= 10
	}
	return strconv.Itoa(magnitude) + "+"
}

// powerOfTwo rounds the value down to one of the sizes, powers of two ending with the one of the larger values.
func powerOfTwo(value uint64, sizes []string) string {
	for i := len(sizes) - 1; i > 0; i-- {
		size, _ := strconv.ParseUint(strings.TrimSuffix(sizes[i], "+"), 10, 64)
		if value >= size {
			return sizes[i]
		}
	}
	return sizes[0]
}

var versionRegexp = regexp.MustCompile("^" + globalRules.Regexps[regexpVersion] + "$")

// versionName returns a version like 2025.1.2, other for anything else, like the custom tag of an image.
func versionName(version string) string {
	if versionRegexp.MatchString(version) {
		return version
	}
	return other
}

// oneOf returns the value if it is one of the values of its enum, other otherwise.
func oneOf(value string, values []string) string {
	if slices.Contains(values, value) {
		return value
	}
	return other
}

// Fail sets the precise reason of a failure, which analysis.finished and finished report instead of any other.
func Fail(reason string) {
	mu.Lock()
	defer mu.Unlock()
	knownReason = reason
}

// SetScenario sets how the analysis runs: on the current state, on a diff of commits, on every commit, or on
// the current state because the start commit of a diff is missing.
func SetScenario(runScenario string) {
	mu.Lock()
	defer mu.Unlock()
	scenario = runScenario
}

// StartAnalysis records analysis.started and returns the function recording analysis.finished, with the reason of
// Finish.
func StartAnalysis() func(exitCode int, reason string) {
	start := time.Now()
	mu.Lock()
	defer mu.Unlock()
	record(groupRun, analysisStarted, nil)
	analyzing = true
	return func(exitCode int, reason string) {
		mu.Lock()
		defer mu.Unlock()
		reason = exitReason(exitCode, reason)
		if reason == ReasonLicenseExpired {
			// a container may have handed it over already
			recordOnce(groupRun, licenseRejected, map[string]any{"reject_reason": "expired"})
		}
		data := map[string]any{
			"exit_code":   exitCode,
			"reason":      reason,
			"duration_ms": time.Since(start).Milliseconds(),
		}
		if scenario != "" {
			data["scenario"] = scenario
		}
		record(groupRun, analysisFinished, data)
	}
}

// Finish records the finished event and sends everything collected. Only the first call has an effect.
// Unless a more precise reason is known, the given reason is reported, or the one of the exit code if it is empty.
func Finish(exitCode int, reason string) {
	finishOnce.Do(
		func() {
			mu.Lock()
			if !enabled {
				mu.Unlock()
				return
			}
			ready, done, started := senders, collected, startedAt
			reason = exitReason(exitCode, reason)
			record(
				groupRun, finished, map[string]any{
					"exit_code":   exitCode,
					"reason":      reason,
					"success":     isSuccess(reason),
					"stage":       cmp.Or(failedStage, stage),
					"duration_ms": time.Since(started).Milliseconds(),
				},
			)
			mu.Unlock()

			// the sender and the environment are awaited without mu: their goroutines track under it
			var s *sender
			if ready != nil {
				deadline, cancel := context.WithDeadline(context.Background(), started.Add(senderDeadline))
				defer cancel()
				if _, ok := receive(done, deadline.Done()); !ok {
					log.Debug("The environment is not collected in time")
				}
				if s, _ = receive(ready, deadline.Done()); s == nil {
					log.Debug("Statistics are not sent: the sender is not ready")
				}
			}
			mu.Lock()
			h := handover{
				Events: events, ProjectId: projectId, OrgId: orgId, Reason: reason, Allowed: allowed,
				StartCommitMissing: scenario == ScenarioStartCommitMissing,
			}
			pending := withIds(events)
			enabled, events = false, nil
			mu.Unlock()
			switch file := handoverFile(); {
			case file != "":
				handOver(file, h)
			case s != nil && h.Allowed:
				if err := s.send(pending); err != nil {
					log.Debugf("Failed to send statistics: %v", err)
				}
			}
		},
	)
}

// receive returns the value of ch, or false when it is not ready by the deadline, which may have passed long ago.
func receive[T any](ch <-chan T, deadline <-chan struct{}) (T, bool) {
	select {
	case value := <-ch:
		return value, true
	default:
	}
	select {
	case value := <-ch:
		return value, true
	case <-deadline:
		var zero T
		return zero, false
	}
}

// exitReason returns the reason reported in analysis.finished and finished events: the precise reason of a failure
// if it is known, otherwise the given one, otherwise the one of the exit code. Must be called under mu.
func exitReason(exitCode int, reason string) string {
	if knownReason != "" || reason != "" {
		return cmp.Or(knownReason, reason)
	}
	switch exitCode {
	case exitcodes.QodanaSuccessExitCode:
		return ReasonSuccess
	case exitcodes.QodanaFailThresholdExitCode:
		return ReasonFailThreshold
	case exitcodes.QodanaEmptyChangesetExitCodePlaceholder:
		return ReasonEmptyChangeset
	case exitcodes.QodanaEapLicenseExpiredExitCode:
		return ReasonLicenseExpired
	case exitcodes.QodanaTimeoutExitCodePlaceholder:
		return ReasonTimeout
	case exitcodes.QodanaOutOfMemoryExitCode:
		return ReasonOutOfMemory
	case exitcodes.QodanaInternalErrorExitCode:
		return ReasonInternalError
	}
	if !analyzing {
		return ReasonSetupError
	}
	return ReasonLinterError
}

func isSuccess(reason string) bool {
	return reason == ReasonSuccess || reason == ReasonFailThreshold || reason == ReasonEmptyChangeset
}

func track(group string, id string, data map[string]any) {
	mu.Lock()
	defer mu.Unlock()
	record(group, id, data)
}

// trackOnce tracks an event the run may come to several times, e.g. for each analyzed commit.
func trackOnce(group string, id string, data map[string]any) {
	mu.Lock()
	defer mu.Unlock()
	recordOnce(group, id, data)
}

// record adds an event of a run collecting statistics. Must be called under mu.
func record(group string, id string, data map[string]any) {
	if !enabled {
		return
	}
	events = append(events, event{group, id, time.Now().UnixMilli(), maps.Clone(data)})
	if group == groupRun {
		stage = id
	}
}

// recordOnce records an event unless the run has it already. Must be called under mu.
func recordOnce(group string, id string, data map[string]any) {
	if !slices.ContainsFunc(events, func(e event) bool { return e.Id == id }) {
		record(group, id, data)
	}
}

// withIds attaches the command, project and organization ids to the run events, and the token source, which a
// container doesn't know, to the license ones. Must be called under mu.
func withIds(pending []event) []event {
	ids := map[string]any{"command": command}
	for key, value := range map[string]string{"project_id": projectId, "org_id": orgId} {
		if value != "" {
			ids[key] = value
		}
	}
	result := make([]event, 0, len(pending))
	for _, e := range pending {
		if e.Group == groupRun {
			data := maps.Clone(ids)
			maps.Copy(data, e.Data)
			if tokenSource != "" && (e.Id == licenseObtained || e.Id == licenseRejected) {
				data["token"] = tokenSource
			}
			e.Data = data
		}
		result = append(result, e)
	}
	return result
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
