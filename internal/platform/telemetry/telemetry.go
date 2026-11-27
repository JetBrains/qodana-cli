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
	"github.com/JetBrains/qodana-cli/internal/platform/version"
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
	// build is the version the statistics are sent with, see SetVersion
	build = version.Version

	mu         sync.Mutex
	enabled    bool
	allowed    = true
	command    string
	projectId  string
	orgId      string
	analysisId string
	scenario   string
	startedAt  time.Time
	analyzing  bool
	// analysisMode, analysisStart and analysisDone are of the analysis StartAnalysis opened, analysisReason is how it
	// ended: a linter container hands it over, apart from the reason of its command
	analysisMode   string
	analysisStart  time.Time
	analysisDone   bool
	analysisReason string
	// collectionStarted tells that Start began collecting, which Finish keeps, unlike enabled
	collectionStarted bool
	// stage is the last funnel step the run reached, failedStage the step a container run failed at
	stage       string
	failedStage string
	tokenSource string
	// baselineKnown tells that the CLI resolved the baseline, hasBaseline that the analysis is given one: an IDE
	// linter resolves its cloud baseline itself
	baselineKnown bool
	hasBaseline   bool
	// linter and setup make linter.ready, which a Docker run knows before the pull and a native one after the download
	linter map[string]any
	setup  map[string]any
	// linterMode is native, docker or third_party, of the analysis events
	linterMode string
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

// Start begins collecting statistics for the command; nothing is collected when the user opted out. repository gives
// the device id the CLI gives the linters it runs and the remote url of the project, it is called only when
// statistics are on. env is given to the linter with --env.
func Start(cmd string, repository func() (deviceId string, remoteUrl string), noStatistics bool, env []string) {
	if disabled || OptedOut(noStatistics) || isLicenseOnly(env) {
		return
	}
	now := time.Now()
	// the CLI that started this container reports the run: nothing is sent from here
	inContainer := handoverFile() != ""
	// repository runs git: not under mu. A container checks the remote too: a third-party linter sends its legacy
	// events from there, see Allowed
	id, remote := repository()
	var ready chan *sender
	var done chan struct{}
	if !inContainer {
		ready, done = make(chan *sender, 1), make(chan struct{})
	}
	endpoint, _ := parseUrl(envValue(env, qdenv.QodanaEndpointEnv))
	jetBrains := isJetBrainsHost(endpoint) || isJetBrainsRepository(remote)
	mu.Lock()
	enabled, collectionStarted, command, startedAt, senders, collected = true, true, cmd, now, ready, done
	allowed = allowed && !jetBrains
	binaryVersion := build
	mu.Unlock()
	log.RegisterExitHandler(func() { Finish(1, ReasonFatal) })

	if !inContainer {
		// the sender fetches the FUS config over the network: prepare it while the command runs
		go func() {
			var s *sender
			defer func() { ready <- s }()
			defer recoverPanic()
			s = connect(id, binaryVersion)
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
		cmd+startedSuffix,
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
// env is given to the linter with --env, overriding the environment of the CLI.
func isLicenseOnly(env []string) bool {
	return envValue(env, qdenv.QodanaLicenseOnlyToken) != "" && envValue(env, qdenv.QodanaToken) == ""
}

// envValue returns the variable of the environment of the CLI or, overriding it, of env.
func envValue(env []string, key string) string {
	value := os.Getenv(key)
	for _, variable := range env {
		if name, envValue, _ := strings.Cut(variable, "="); name == key {
			value = envValue
		}
	}
	return value
}

// SetVersion sets the version of a binary built without the one of the CLI, qodana-clang and qodana-cdnet. It must
// be called before Start.
func SetVersion(binaryVersion string) {
	mu.Lock()
	defer mu.Unlock()
	build = binaryVersion
}

// Allow permits sending the statistics, it is forbidden for license-only tokens and internal organizations of
// JetBrains.
func Allow(isAllowed bool) {
	mu.Lock()
	defer mu.Unlock()
	allowed = allowed && isAllowed
}

// Allowed reports whether the statistics of this run may be sent: not when the user opted out, the run is air-gapped
// or has a license-only token, nor for a run of JetBrains: its repository, its Qodana Cloud endpoint or an internal
// organization. It is final when the command finishes.
func Allowed() bool {
	mu.Lock()
	defer mu.Unlock()
	return collectionStarted && allowed
}

// SetCloudIds sets the Qodana Cloud project and organization id hashes attached to every run event.
func SetCloudIds(projectIdHash string, organizationIdHash string) {
	mu.Lock()
	defer mu.Unlock()
	projectId, orgId = projectIdHash, organizationIdHash
}

var analysisIdRegexp = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// AnalysisId returns the analysis id the statistics are sent with, empty unless it is a UUID: --analysis-id takes
// any text, like a CI build number, and the IDE rejects it then with the same pattern, see QodanaEventsData.
func AnalysisId(id string) string {
	if analysisIdRegexp.MatchString(id) {
		return id
	}
	return ""
}

// SetAnalysisId sets the analysis id attached to every run event, see AnalysisId.
func SetAnalysisId(id string) {
	mu.Lock()
	defer mu.Unlock()
	analysisId = AnalysisId(id)
}

// LinterReady records the linter the analysis runs with: mode is native, docker, or third_party for qodana-clang
// and qodana-cdnet. linter.ready is recorded once the linter is on the machine too, see LinterPrepared: a native
// linter is downloaded before, a Docker image is pulled after.
func LinterReady(code string, version string, paid bool, eap bool, mode string) {
	// eap tells an EAP build
	version, _, _ = strings.Cut(version, "-")
	if version != "latest" {
		version = versionName(version)
	}
	mu.Lock()
	defer mu.Unlock()
	linter, linterMode = map[string]any{"linter": code, "version": version, "paid": paid, "eap": eap}, mode
	switch {
	case mode == "third_party":
		setup = map[string]any{"setup": SetupThirdParty, "download_ms": 0}
	case mode == "native" && setup == nil:
		// a linter image, or a native linter given with its path
		setup = map[string]any{"setup": SetupNativePreinstalled, "download_ms": 0}
	}
	// a Qodana image the user runs, like in a CI template: a container this CLI starts hands its events over instead
	if mode == "native" && qdenv.IsContainer() {
		linterMode = "container"
	}
	recordLinterReady()
}

// LinterPrepared records how the linter came on the machine, see the Setup constants, and how long it took.
func LinterPrepared(how string, duration time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	setup = map[string]any{"setup": how, "download_ms": duration.Milliseconds()}
	recordLinterReady()
}

// recordLinterReady records linter.ready once the linter and its setup are known. Must be called under mu.
func recordLinterReady() {
	if linter == nil || setup == nil {
		return
	}
	data := maps.Clone(linter)
	maps.Copy(data, setup)
	// a full-history run pulls the image for each commit
	recordOnce(groupRun, linterReady, data)
}

// LinterFailed records that the native linter can't be downloaded, and the reason the run fails with: its version is
// unknown before the download.
func LinterFailed(code string) {
	mu.Lock()
	defer mu.Unlock()
	recordFailure(linterFailed, map[string]any{"linter": code}, ReasonDownloadFailed)
}

// DockerFailed records that the Docker run of the linter can't start, see dockerReasons, and the reason the run
// fails with. The linter is the one LinterReady knows.
func DockerFailed(reason string) {
	mu.Lock()
	defer mu.Unlock()
	data := map[string]any{"reason": reason}
	if code, ok := linter["linter"]; ok {
		data["linter"] = code
	}
	recordFailure(dockerFailed, data, reason)
}

func LicenseObtained(kind string, plan string) {
	data := map[string]any{"kind": kind}
	if plan != "" {
		data["plan"] = oneOf(plan, licensePlans)
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
	recordFailure(licenseRejected, map[string]any{"reason": reason}, ReasonLicenseRejected)
}

// BaselineResolved records the baseline the analysis compares its results with, a local file or the one of Qodana
// Cloud: found, none on purpose, or why it can't be used. httpStatus is the answer of a failed Qodana Cloud request.
func BaselineResolved(source string, status string, httpStatus int, downloadTime time.Duration) {
	data := map[string]any{"source": source}
	id := baselineFailed
	switch {
	case status == "found":
		id = baselineFound
	case slices.Contains(noBaselineReasons, status):
		// the reason tells the source
		id, data = baselineNone, map[string]any{}
	}
	if id != baselineFound {
		data["reason"] = status
	}
	if httpStatus != 0 {
		data["http_status"] = httpStatus
	}
	if downloadTime != 0 {
		data["request_ms"] = downloadTime.Milliseconds()
	}
	mu.Lock()
	defer mu.Unlock()
	// a missing baseline file is no baseline: problems.found reports baseline=none
	baselineKnown, hasBaseline = true, id == baselineFound
	record(groupRun, id, data)
}

// ReportPublished records whether the report reached Qodana Cloud; it is not published without an upload token.
func ReportPublished(hasToken bool, uploaded bool) {
	mu.Lock()
	defer mu.Unlock()
	// the CLI of a linter container records it too and hands it over: the first one is kept
	if slices.ContainsFunc(events, func(e event) bool { return e.Id == reportPublished || e.Id == publishFailed }) {
		return
	}
	switch {
	case uploaded:
		record(groupRun, reportPublished, nil)
	case hasToken:
		record(groupRun, publishFailed, nil)
	}
}

// ReportShown records where qodana show shows the report: the one of Qodana Cloud, the local one it serves, or the
// results directory.
func ReportShown(target string) {
	track(groupRun, reportShown, map[string]any{"target": target})
}

// ShowFailed records that qodana show can't show the report, and the reason the run fails with.
func ShowFailed(reason string) {
	mu.Lock()
	defer mu.Unlock()
	recordFailure(showFailed, map[string]any{"reason": reason}, reason)
}

// ContributorsCounted records the active contributors qodana contributors counts in the repositories over the days,
// by order of magnitude.
func ContributorsCounted(contributors int, projectDirs int, days int) {
	track(
		groupRun, contributorsCounted, map[string]any{
			"contributors": bucket(contributors), "project_dirs": bucket(projectDirs), "days": max(days, 0),
		},
	)
}

// LinterSelected records the linter qodana init configures the project with, or that the project has one already,
// and its mode: native or docker, empty when qodana scan chooses it.
func LinterSelected(code string, preconfigured bool, mode string) {
	data := map[string]any{"preconfigured": preconfigured}
	if code != "" {
		data["linter"] = code
	}
	if mode != "" {
		data["mode"] = mode
	}
	track(groupRun, linterSelected, data)
}

// DockerReady records the container engine the linter runs in: its limits, not the host ones, apply to the linter.
// It is a step of a Docker run, recorded when the engine runs Linux.
func DockerReady(cpuCount int, memoryBytes int64, engineVersion string) {
	// a distribution build like 20.10.24+dfsg1 or 26.1.4-ce is reported by its version
	if i := strings.IndexAny(engineVersion, "+-"); i >= 0 {
		engineVersion = engineVersion[:i]
	}
	trackOnce(
		groupRun, dockerReady, map[string]any{
			"cpu_count": powerOfTwo(uint64(cpuCount), cpuCounts),
			"ram_gb":    powerOfTwo(uint64(memoryBytes/gb), memorySizes),
			"version":   versionName(engineVersion),
		},
	)
}

// ProblemsFound records the numbers of problems of the report by their SARIF baseline states, in buckets, and
// whether the analysis applied the baseline it was given: only then the problems have baseline states. Absent
// problems are of the baseline, not of the project. A baseline which an IDE linter resolves itself is told by the
// states only: applied, or none without them.
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
	case baselineKnown && !hasBaseline:
		data["baseline"] = "none"
	case total+countsByBaselineState["absent"] == 0:
		// a report without problems does not tell
		data["baseline"] = "unknown"
	case countsByBaselineState[""] == 0:
		data["baseline"] = "applied"
	case !baselineKnown:
		data["baseline"] = "none"
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

// Fail sets the precise reason of a failure, which the failed events of the analysis and the command report instead
// of any other.
func Fail(reason string) {
	mu.Lock()
	defer mu.Unlock()
	knownReason = reason
}

// ConfigResolved records where the Qodana configuration comes from: the qodana.yaml of the project, a global
// configuration, both or none.
func ConfigResolved(local bool, global bool) {
	source := "none"
	switch {
	case local && global:
		source = "local_and_global"
	case local:
		source = "local"
	case global:
		source = "global"
	}
	trackOnce(groupRun, configResolved, map[string]any{"source": source})
}

// ConfigFailed records that the Qodana configuration can't be loaded, and the reason the run fails with.
func ConfigFailed() {
	mu.Lock()
	defer mu.Unlock()
	recordFailure(configFailed, nil, ReasonInvalidConfig)
}

// GitPrepared records whether the start commit of an incremental or full-history analysis is in the repository.
// Without it the analysis runs on the current state, see ScenarioStartCommitMissing.
func GitPrepared(startCommitFound bool) {
	mu.Lock()
	defer mu.Unlock()
	startCommit := "found"
	if !startCommitFound {
		startCommit, scenario = "missing", ScenarioStartCommitMissing
	}
	recordOnce(groupRun, gitPrepared, map[string]any{"start_commit": startCommit})
}

// GitFailed records that the analysis needs git, which is not installed, and the reason the run fails with.
func GitFailed() {
	mu.Lock()
	defer mu.Unlock()
	recordFailure(gitFailed, nil, ReasonGitMissing)
}

// BootstrapFinished records the bootstrap command and, if it failed, the reason the run fails with. A diff runs it
// for both commits, the first run is recorded.
func BootstrapFinished(exitCode int, duration time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	if exitCode == 0 {
		recordOnce(groupRun, bootstrapSucceeded, map[string]any{"duration_ms": duration.Milliseconds()})
		return
	}
	recordFailure(
		bootstrapFailed,
		map[string]any{"exit_code": exitCode, "duration_ms": duration.Milliseconds()},
		ReasonBootstrapFailed,
	)
}

// recordFailure records the failed event of a step, once, the step as the stage the run ends at, and the reason the
// run fails with. Must be called under mu.
func recordFailure(id string, data map[string]any, reason string) {
	recordOnce(groupRun, id, data)
	failedStage, knownReason = id, reason
}

// StartAnalysis records analysis.started and returns the function recording analysis.succeeded or analysis.failed,
// with the reason of Finish. runScenario is the one the options ask for: default, scope, reversed-scope or
// full-history, empty for a linter without a diff.
func StartAnalysis(runScenario string) func(exitCode int, reason string) {
	mu.Lock()
	defer mu.Unlock()
	scenario, analysisMode, analysisStart, analysisDone, analysisReason =
		runScenario, scanMode(runScenario), time.Now(), false, ""
	record(groupRun, analysisStarted, map[string]any{"scan_mode": analysisMode})
	analyzing = true
	return func(exitCode int, reason string) {
		mu.Lock()
		defer mu.Unlock()
		// a linter container tells how its analysis ended: a failure after it, like the upload, isn't of the analysis
		finishAnalysis(exitCode, cmp.Or(analysisReason, exitReason(exitCode, reason)))
	}
}

// finishAnalysis records analysis.succeeded or analysis.failed of the open analysis, once. Finish closes the one a
// log.Fatal or an os.Exit left open, with the reason of the command. Must be called under mu.
func finishAnalysis(exitCode int, reason string) {
	if !analyzing || analysisDone {
		return
	}
	analysisDone = true
	if reason == ReasonLicenseExpired {
		// a container may have handed it over already
		recordOnce(groupRun, licenseRejected, map[string]any{"reason": "expired"})
	}
	data := map[string]any{
		"exit_code":   exitCode,
		"duration_ms": time.Since(analysisStart).Milliseconds(),
		"scan_mode":   analysisMode,
	}
	if analysisMode == scanIncremental {
		data["scenario"] = scenario
	}
	// the success rate and the duration per linter and mode need no join with linter.ready
	if code, ok := linter["linter"]; ok {
		data["linter"], data["mode"] = code, linterMode
	}
	analysisReason = reason
	if !isSuccess(reason) {
		reason = oneOf(reason, analysisReasons)
	}
	record(resultEvent(analysisSucceeded, analysisFailed, reason, data))
}

// scanMode gives the scan_mode of a run scenario.
func scanMode(runScenario string) string {
	switch runScenario {
	case "scope", "reversed-scope":
		return scanIncremental
	case "full-history":
		return scanFullHistory
	}
	return scanFull
}

// resultEvent gives the succeeded event with the result, or the failed one with the reason.
func resultEvent(
	succeeded string, failure string, reason string, data map[string]any,
) (string, string, map[string]any) {
	if isSuccess(reason) {
		data["result"] = reason
		return groupRun, succeeded, data
	}
	data["reason"] = reason
	return groupRun, failure, data
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
			finishAnalysis(exitCode, reason)
			record(
				resultEvent(
					commandSucceeded, commandFailed, reason, map[string]any{
						"exit_code":   exitCode,
						"stage":       cmp.Or(failedStage, stage),
						"duration_ms": time.Since(started).Milliseconds(),
					},
				),
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
				AnalysisReason: analysisReason,
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

// exitReason returns the reason of the result events of the analysis and the command: the precise reason of a failure
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

// withIds attaches the project, organization and analysis ids to the run events, the command to the finish ones, and
// the token source, which a container doesn't know, to the license ones. Must be called under mu.
func withIds(pending []event) []event {
	ids := map[string]any{}
	for key, value := range map[string]string{"project_id": projectId, "org_id": orgId, "analysis_id": analysisId} {
		if value != "" {
			ids[key] = value
		}
	}
	result := make([]event, 0, len(pending))
	for _, e := range pending {
		if e.Group == groupRun {
			data := maps.Clone(ids)
			maps.Copy(data, e.Data)
			if e.Id == commandSucceeded || e.Id == commandFailed {
				data["command"] = command
			}
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
