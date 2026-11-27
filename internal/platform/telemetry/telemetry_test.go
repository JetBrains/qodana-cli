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

package telemetry

import (
	"cmp"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	fus "github.com/JetBrains/fus-reporting-api-go"
	"github.com/JetBrains/qodana-cli/internal/core/exitcodes"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/JetBrains/qodana-cli/internal/platform/version"
	cienvironment "github.com/cucumber/ci-environment/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestValidator(t *testing.T) *fus.Validator {
	scheme, err := definition.BuildValidationScheme()
	require.NoError(t, err)
	validator, err := fus.NewValidator(scheme)
	require.NoError(t, err)
	return validator
}

func TestEventsMatchScheme(t *testing.T) {
	validator := newTestValidator(t)
	ids := map[string]any{"project_id": testProject, "org_id": testOrg}
	system, software := environment()
	testEvents := []event{
		{
			groupRun,
			"scan" + startedSuffix,
			0,
			map[string]any{
				"ci": "github-actions", "container": false, "interactive": true,
			},
		},
		{groupRun, configResolved, 0, map[string]any{"source": "local_and_global"}},
		{groupRun, configFailed, 0, map[string]any{}},
		{groupRun, gitPrepared, 0, map[string]any{"start_commit": "missing"}},
		{groupRun, gitFailed, 0, map[string]any{}},
		{groupRun, bootstrapSucceeded, 0, map[string]any{"duration_ms": int64(3000)}},
		{
			groupRun,
			bootstrapFailed,
			0,
			map[string]any{"exit_code": 127, "duration_ms": int64(10)},
		},
		{groupRun, licenseObtained, 0, map[string]any{"kind": "cloud", "plan": "ULTIMATE_PLUS"}},
		{groupRun, licenseRejected, 0, map[string]any{"reason": "token_declined"}},
		{
			groupRun,
			linterReady,
			0,
			map[string]any{
				"linter": "QDJVM", "version": "2025.3", "paid": true, "eap": false,
				"setup": SetupNativeDownloaded, "download_ms": int64(42000),
			},
		},
		{
			groupRun,
			linterReady,
			0,
			map[string]any{
				"linter": "QDPYC", "version": "latest", "paid": false, "eap": true,
				"setup": SetupDockerPullSkipped, "download_ms": int64(0),
			},
		},
		{
			groupRun,
			linterFailed,
			0,
			map[string]any{"linter": "QDGO"},
		},
		{groupRun, dockerReady, 0, map[string]any{"cpu_count": "8", "ram_gb": "4", "version": "27.3.1"}},
		{groupRun, dockerFailed, 0, map[string]any{"linter": "QDGO", "reason": ReasonPullFailed}},
		{groupRun, baselineFound, 0, map[string]any{"source": "cloud", "request_ms": int64(250)}},
		{groupRun, baselineNone, 0, map[string]any{"reason": "not_connected"}},
		{
			groupRun,
			baselineFailed,
			0,
			map[string]any{"source": "cloud", "reason": "request_failed", "http_status": 403, "request_ms": int64(25)},
		},
		{groupRun, analysisStarted, 0, map[string]any{"scan_mode": scanIncremental}},
		{
			groupRun,
			analysisSucceeded,
			0,
			map[string]any{
				"exit_code": 255, "result": ReasonFailThreshold, "duration_ms": int64(1234), "scan_mode": scanFull,
				"linter": "QDJVM", "mode": "docker",
			},
		},
		{
			groupRun,
			analysisFailed,
			0,
			map[string]any{
				"exit_code":   1,
				"reason":      ReasonLinterError,
				"duration_ms": int64(1234),
				"scan_mode":   scanIncremental,
				"scenario":    "reversed-scope",
			},
		},
		{groupRun, linterSelected, 0, map[string]any{"linter": "QDNETC", "mode": "native", "preconfigured": false}},
		{groupRun, licenseRejected, 0, map[string]any{"reason": "expired", "token": "keychain"}},
		{
			groupRun,
			problemsFound,
			0,
			map[string]any{"total": "100+", "new": "10+", "unchanged": "100+", "absent": "0", "baseline": "applied"},
		},
		{groupRun, reportPublished, 0, map[string]any{}},
		{groupRun, publishFailed, 0, map[string]any{}},
		{groupRun, reportShown, 0, map[string]any{"target": "local"}},
		{groupRun, showFailed, 0, map[string]any{"reason": ReasonReportMissing}},
		{groupRun, contributorsCounted, 0, map[string]any{"contributors": "10+", "project_dirs": "1+", "days": 90}},
		{
			groupRun,
			commandSucceeded,
			0,
			map[string]any{
				"command": "scan", "exit_code": 0, "result": ReasonSuccess, "stage": problemsFound, "duration_ms": int64(5),
			},
		},
		{
			groupRun,
			commandFailed,
			0,
			map[string]any{
				"command": "scan", "exit_code": -1, "reason": ReasonPullFailed, "stage": dockerFailed,
				"duration_ms": int64(5),
			},
		},
		{groupEnv, "system", 0, system},
		{groupEnv, "software", 0, software},
	}
	for _, e := range testEvents {
		t.Run(
			e.Group+"/"+e.Id, func(t *testing.T) {
				data := maps.Clone(e.Data)
				if e.Group == groupRun {
					maps.Copy(data, ids)
				}
				assert.LessOrEqual(t, len(data), fus.MaxDataFields)
				validated, dropped := validator.Validate(
					fus.LogEvent{
						Group: fus.Group(e.Group, 1),
						Event: fus.EventAction{ID: e.Id, Data: data},
					},
				)
				require.False(t, dropped)
				assert.Equal(t, e.Id, validated.Event.ID)
				for key, value := range validated.Event.Data {
					if s, ok := value.(string); ok {
						assert.False(
							t,
							strings.HasPrefix(s, "validation.") || s == "undefined",
							"%s=%v is rejected",
							key,
							e.Data[key],
						)
					}
				}
			},
		)
	}
}

func TestCommandsMatchScheme(t *testing.T) {
	validator := newTestValidator(t)
	for _, command := range []string{"init", "scan", "send", "show", "contributors"} {
		data := map[string]any{
			"command": command, "exit_code": 0, "result": ReasonSuccess, "stage": command + startedSuffix,
		}
		validated, dropped := validator.Validate(
			fus.LogEvent{Group: fus.Group(groupRun, 1), Event: fus.EventAction{ID: commandSucceeded, Data: data}},
		)
		require.False(t, dropped, command)
		assert.Equal(t, command, validated.Event.Data["command"])
		assert.Equal(t, command+startedSuffix, validated.Event.Data["stage"])
	}
}

func TestReasonsMatchScheme(t *testing.T) {
	validator := newTestValidator(t)
	for _, reason := range reasons {
		data := map[string]any{"command": "show", "exit_code": 1, "reason": reason, "stage": "show" + startedSuffix}
		validated, dropped := validator.Validate(
			fus.LogEvent{Group: fus.Group(groupRun, 1), Event: fus.EventAction{ID: commandFailed, Data: data}},
		)
		require.False(t, dropped, reason)
		assert.Equal(t, reason, validated.Event.Data["reason"])
	}
}

func TestMissingReportIsNoCrash(t *testing.T) {
	received := fakeFus(t)
	Start("show", func() (string, string) { return testDevice, "" }, false, nil)
	ShowFailed(ReasonReportMissing)
	// the log.Fatal of ShowReport
	Finish(1, ReasonFatal)

	report := <-received
	assert.Equal(t, ReasonReportMissing, lastEvent(report, showFailed)["reason"])
	assert.Equal(t, ReasonReportMissing, lastEvent(report, commandFailed)["reason"])
	assert.Equal(t, showFailed, lastEvent(report, commandFailed)["stage"])
}

func TestShowIsSent(t *testing.T) {
	received := fakeFus(t)
	Start("show", func() (string, string) { return testDevice, "" }, false, nil)
	ReportShown("cloud")
	Finish(0, "")

	report := <-received
	assert.Equal(t, map[string]any{"target": "cloud"}, lastEvent(report, reportShown))
	assert.Equal(t, reportShown, lastEvent(report, commandSucceeded)["stage"])
	assert.Equal(t, "show", lastEvent(report, commandSucceeded)["command"])
}

func TestContributorsAreCounted(t *testing.T) {
	received := fakeFus(t)
	Start("contributors", func() (string, string) { return testDevice, "" }, false, nil)
	ContributorsCounted(42, 3, 90)
	ContributorsCounted(0, 1, -1)
	Finish(0, "")

	report := <-received
	var counted []map[string]any
	for _, e := range report.Events {
		if e.Event.ID == contributorsCounted {
			counted = append(counted, e.Event.Data)
		}
	}
	require.Len(t, counted, 2)
	assert.Equal(
		t,
		map[string]any{"contributors": "10+", "project_dirs": "1+", "days": float64(90)},
		counted[0],
	)
	assert.Equal(t, float64(0), counted[1]["days"], "a negative period is no period")
}

// ids as the linter and Qodana Cloud have them
const (
	testDevice  = "200820300000000-9bfa-5df8-1c66-3a9e6438608a"
	testProject = "5e3c9a1b-2f7d-3c44-8a1e-9b0d6f2c4e71"
	testOrg     = "a3f09c2e71b4d856"
)

func TestSendKeepsEventTimeAndIds(t *testing.T) {
	var report fus.Report
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				assert.NoError(t, json.Unmarshal(body, &report))
			},
		),
	)
	defer server.Close()
	s := &sender{
		client:    fus.NewClient(server.URL, sendTimeout, "test"),
		validator: newTestValidator(t),
		base:      fus.LogEvent{Session: "session"},
	}

	err := s.send(
		[]event{
			{
				groupRun,
				analysisStarted,
				1700000000000,
				map[string]any{"command": "scan", "project_id": testProject},
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, report.Events, 1)
	assert.Equal(t, int64(1700000000000), report.Events[0].Time)
	assert.Equal(t, "session", report.Events[0].Session)
	assert.Equal(t, testProject, report.Events[0].Event.Data["project_id"], "the ids are hashed already")
}

func TestPostHogGetsTheEventsOfFus(t *testing.T) {
	var batch struct {
		ApiKey string         `json:"api_key"`
		Batch  []postHogEvent `json:"batch"`
	}
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/batch/", r.URL.Path)
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&batch))
			},
		),
	)
	defer server.Close()
	host, key := postHogHost, postHogKey
	postHogHost, postHogKey = server.URL, "phc_test"
	t.Cleanup(func() { postHogHost, postHogKey = host, key })
	fusServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer fusServer.Close()
	s := &sender{
		client:    fus.NewClient(fusServer.URL, sendTimeout, "test"),
		postHog:   true,
		validator: newTestValidator(t),
		base:      fus.LogEvent{IDs: map[string]string{"device": testDevice}, Session: "session", Build: "2026.1"},
	}

	err := s.send(
		[]event{
			{
				groupRun,
				licenseObtained,
				1700000000000,
				map[string]any{
					"command":    "scan",
					"kind":       "cloud",
					"plan":       "ULTIMATE",
					"project_id": testProject,
					"org_id":     testOrg,
				},
			},
			{
				groupRun,
				licenseRejected,
				1700000000001,
				map[string]any{"command": "scan", "reason": "invented", "org_id": "org hash"},
			},
			{groupEnv, "system", 1700000000002, map[string]any{"os": "linux"}},
			{groupRun, dockerReady, 1700000000003, map[string]any{"version": "27.3.1"}},
		},
	)

	require.NoError(t, err)
	assert.Equal(t, "phc_test", batch.ApiKey)
	require.Len(t, batch.Batch, 4)
	license := batch.Batch[0]
	assert.Equal(t, "qd.cli.run.license.obtained", license.Event)
	assert.Equal(t, testDevice, license.DistinctId, "the subject is the device, stable per repository")
	assert.Equal(t, false, license.Properties["$process_person_profile"])
	assert.Equal(t, time.UnixMilli(1700000000000).UTC(), license.Timestamp)
	assert.Equal(
		t,
		map[string]any{"organization": testOrg, "project": testProject, "repository": testDevice},
		license.Properties["$groups"],
	)
	assert.Equal(t, "ULTIMATE", license.Properties["plan"])
	assert.Equal(t, "session", license.Properties["session"])
	rejected := batch.Batch[1]
	assert.NotEqual(t, "invented", rejected.Properties["reason"], "PostHog gets the values FUS accepts")
	assert.NotContains(t, rejected.Properties, "org_id", "a rejected id is no id")
	assert.Equal(t, map[string]any{"repository": testDevice}, rejected.Properties["$groups"])
	assert.Equal(t, testDevice, batch.Batch[2].DistinctId, "environment events are of the device of the run")
	assert.Equal(t, "2026.1", batch.Batch[3].Properties["cli_version"])
	assert.Equal(
		t,
		"27.3.1",
		batch.Batch[3].Properties["version"],
		"the CLI version doesn't replace the one of the event",
	)
}

func TestContainerHandsTheRunOver(t *testing.T) {
	cacheDir := t.TempDir()
	// the CLI in the container
	fakeFus(t)
	t.Setenv(handoverEnv, filepath.Join(cacheDir, handoverName))
	Start("scan", func() (string, string) { return testDevice, "" }, false, nil)
	LicenseObtained("cloud", "ULTIMATE")
	GitPrepared(false)
	Allow(false)
	Finish(1, ReasonLinterError)
	// the CLI on the host
	reset(t)
	enabled, scenario = true, "scope"

	CollectFromContainer(cacheDir)

	assert.Equal(t, ScenarioStartCommitMissing, scenario)
	require.Len(t, events, 2, "the steps the container knows, not its started event")
	assert.Equal(t, licenseObtained, events[0].Id)
	assert.Equal(t, gitPrepared, events[1].Id)
	assert.Equal(t, ReasonLinterError, exitReason(1, ""))
	assert.False(t, allowed, "a container forbidden to send forbids the run")
}

func TestContainerReportsOnlyAMissingStartCommit(t *testing.T) {
	for requested, expected := range map[string]map[bool]string{
		// a full-history host runs the loop: each container runs a single revision, a default or scope run
		"full-history": {false: "full-history", true: ScenarioStartCommitMissing},
		"scope":        {false: "scope", true: ScenarioStartCommitMissing},
	} {
		for missing, scenarioReported := range expected {
			cacheDir := t.TempDir()
			startCommit := map[bool]string{false: "found", true: "missing"}[missing]
			handOver(
				filepath.Join(cacheDir, handoverName), handover{
					Allowed: true,
					Events:  []event{{groupRun, gitPrepared, 1, map[string]any{"start_commit": startCommit}}},
				},
			)
			reset(t)
			enabled, scenario = true, requested

			CollectFromContainer(cacheDir)

			assert.Equal(t, scenarioReported, scenario, "%s, start commit missing: %v", requested, missing)
		}
	}
}

func TestContainerHandover(t *testing.T) {
	cacheDir := t.TempDir()
	file := filepath.Join(cacheDir, handoverName)
	handOver(
		file, handover{
			Events: []event{
				{groupRun, "scan" + startedSuffix, 1, nil},
				{groupRun, licenseRejected, 2, map[string]any{"reason": "token_declined"}},
			},
			ProjectId: "project",
			Reason:    ReasonLicenseRejected,
			Allowed:   true,
		},
	)
	reset(t)
	enabled = true

	CollectFromContainer(cacheDir)

	assert.Equal(t, []event{{groupRun, licenseRejected, 2, map[string]any{"reason": "token_declined"}}}, events)
	assert.Equal(t, "project", projectId)
	assert.Equal(t, ReasonLicenseRejected, exitReason(1, ""))
	assert.NoFileExists(t, file)
}

func TestRepeatedContainersAreCountedOnce(t *testing.T) {
	cacheDir := t.TempDir()
	reset(t)
	enabled = true
	license := event{groupRun, licenseObtained, 1, map[string]any{"kind": "cloud"}}
	for _, reason := range []string{ReasonOutOfMemory, ReasonSuccess} {
		handOver(
			filepath.Join(cacheDir, handoverName),
			handover{Events: []event{license}, Reason: reason, Allowed: true},
		)
		CollectFromContainer(cacheDir)
	}

	assert.Equal(t, []event{license}, events)
	assert.Equal(t, ReasonSuccess, exitReason(0, ""), "the success of the last container is no failure")
	assert.Equal(t, ReasonInterrupted, exitReason(0, ReasonInterrupted))
}

func TestLicenseRejectedInContainerIsTheStage(t *testing.T) {
	cacheDir := t.TempDir()
	reset(t)
	enabled = true
	track(groupRun, analysisStarted, nil)
	handOver(
		filepath.Join(cacheDir, handoverName), handover{
			Events: []event{{groupRun, licenseRejected, 1, map[string]any{"reason": "token_declined"}}},
			Reason: ReasonLicenseRejected,
		},
	)

	CollectFromContainer(cacheDir)
	track(groupRun, analysisSucceeded, nil)
	Finish(1, "")

	assert.Equal(t, licenseRejected, cmp.Or(failedStage, stage))
}

func TestExpiredLicenseIsRejected(t *testing.T) {
	reset(t)
	enabled = true

	StartAnalysis("")(exitcodes.QodanaEapLicenseExpiredExitCode, "")

	assert.True(
		t, slices.ContainsFunc(
			events, func(e event) bool {
				return e.Id == licenseRejected && e.Data["reason"] == "expired"
			},
		),
	)
}

func TestGivenReasonKeepsTheKnownOne(t *testing.T) {
	reset(t)
	enabled = true

	StartAnalysis("")(0, ReasonEmptyChangeset)
	Fail(ReasonLicenseRejected)
	StartAnalysis("")(3, ReasonTimeout)

	assert.Equal(t, ReasonEmptyChangeset, events[1].Data["result"])
	assert.Equal(t, ReasonLicenseRejected, events[3].Data["reason"], "a container may have handed a failure over")
}

func TestExpiredLicenseInContainerIsNotTheStage(t *testing.T) {
	cacheDir := t.TempDir()
	reset(t)
	enabled = true
	track(groupRun, analysisStarted, nil)
	handOver(
		filepath.Join(cacheDir, handoverName), handover{
			Events: []event{{groupRun, licenseRejected, 1, map[string]any{"reason": "expired"}}},
			Reason: ReasonLicenseExpired,
		},
	)

	CollectFromContainer(cacheDir)
	track(groupRun, analysisFailed, nil)

	assert.Equal(t, analysisFailed, cmp.Or(failedStage, stage), "as in a native run")
}

func TestProblemsFound(t *testing.T) {
	reset(t)
	enabled = true

	ProblemsFound(map[string]int{"": 7})
	BaselineResolved("cloud", "found", 0, 0)
	ProblemsFound(map[string]int{"new": 3, "unchanged": 20, "absent": 500})
	ProblemsFound(map[string]int{})
	ProblemsFound(map[string]int{"": 7})

	assert.Equal(t, "none", events[0].Data["baseline"])
	assert.Equal(
		t, map[string]any{"total": "10+", "new": "1+", "unchanged": "10+", "absent": "100+", "baseline": "applied"},
		events[2].Data, "absent problems are not of the project",
	)
	assert.Equal(t, "unknown", events[3].Data["baseline"], "no problems do not tell")
	assert.Equal(t, "ignored", events[4].Data["baseline"])
}

func TestProblemsFoundWithTheBaselineOfTheLinter(t *testing.T) {
	reset(t)
	enabled = true

	// an IDE linter resolves its cloud baseline itself: the CLI records no baseline.resolved
	ProblemsFound(map[string]int{"new": 3, "unchanged": 20})
	ProblemsFound(map[string]int{"": 7})
	ProblemsFound(map[string]int{})

	assert.Equal(t, "applied", events[0].Data["baseline"])
	assert.Equal(t, "none", events[1].Data["baseline"], "problems without states had no baseline")
	assert.Equal(t, "unknown", events[2].Data["baseline"])
}

func TestTokenSourceIsTheFirstOne(t *testing.T) {
	reset(t)

	SetTokenSource("org_token")
	SetTokenSource("env")

	assert.Equal(t, "org_token", tokenSource, "the exchanged token is in the environment afterwards")
}

func TestValuesOutsideEnumsAreOther(t *testing.T) {
	assert.Equal(t, "ULTIMATE", oneOf("ULTIMATE", licensePlans))
	assert.Equal(t, other, oneOf("STARTER", licensePlans))
	assert.Equal(t, "opensuse", distro("opensuse-leap"))
	assert.Equal(t, other, distro("gentoo"))
}

func TestVersionName(t *testing.T) {
	for raw, expected := range map[string]string{
		"2025.1": "2025.1",
		"27.3.1": "27.3.1",
		"24":     "24",
		"my":     other,
		"":       other,
		"1..2":   other,
	} {
		assert.Equal(t, expected, versionName(raw), raw)
	}
	reset(t)
	enabled = true

	LinterReady("QDJVM", "my-build", true, false, "docker")
	assert.Equal(t, other, linter["version"])
	LinterReady("QDJVM", "latest", true, false, "docker")
	assert.Equal(t, "latest", linter["version"])
}

func TestDockerLinterIsReadyAfterThePull(t *testing.T) {
	reset(t)
	enabled = true

	LinterReady("QDJVM", "2025.3", true, false, "docker")
	assert.Empty(t, events, "the image is not pulled yet")
	LinterPrepared(SetupDockerCached, 2*time.Second)
	// a full-history run pulls the image for each commit
	LinterPrepared(SetupDockerPulled, time.Second)

	require.Len(t, events, 1)
	assert.Equal(
		t, map[string]any{
			"linter": "QDJVM", "version": "2025.3", "paid": true, "eap": false,
			"setup": SetupDockerCached, "download_ms": int64(2000),
		}, events[0].Data,
	)
}

func TestNativeLinterIsReadyAfterTheDownload(t *testing.T) {
	for name, tc := range map[string]struct {
		prepare func()
		mode    string
		setup   string
	}{
		"downloaded":   {func() { LinterPrepared(SetupNativeDownloaded, time.Second) }, "native", SetupNativeDownloaded},
		"preinstalled": {func() {}, "native", SetupNativePreinstalled},
		"third-party":  {func() {}, "third_party", SetupThirdParty},
	} {
		t.Run(name, func(t *testing.T) {
			reset(t)
			enabled = true

			tc.prepare()
			LinterReady("QDGO", "2025.3", true, false, tc.mode)

			require.Len(t, events, 1)
			assert.Equal(t, linterReady, events[0].Id)
			assert.Equal(t, tc.setup, events[0].Data["setup"])
		})
	}
}

func TestDockerFailedIsTheReason(t *testing.T) {
	received := fakeFus(t)
	Start("scan", func() (string, string) { return testDevice, "" }, false, nil)
	LinterReady("QDJVM", "2025.3", true, false, "docker")
	DockerFailed(ReasonPullFailed)
	Finish(1, "")

	report := <-received
	assert.Equal(t, map[string]any{"linter": "QDJVM", "reason": ReasonPullFailed}, lastEvent(report, dockerFailed))
	assert.Nil(t, lastEvent(report, linterReady))
	assert.Equal(t, ReasonPullFailed, lastEvent(report, commandFailed)["reason"])
	assert.Equal(t, dockerFailed, lastEvent(report, commandFailed)["stage"])
}

func TestNativeDownloadFailedIsTheReason(t *testing.T) {
	reset(t)
	enabled = true

	LinterFailed("QDGO")

	assert.Equal(t, []event{{groupRun, linterFailed, events[0].Time, map[string]any{"linter": "QDGO"}}}, events)
	assert.Equal(t, ReasonDownloadFailed, knownReason)
}

func TestBaselineEvents(t *testing.T) {
	for status, id := range map[string]string{
		"found": baselineFound, "not_connected": baselineNone, "disabled": baselineNone, "empty": baselineNone,
		"missing": baselineFailed, "unsupported": baselineFailed, "timeout": baselineFailed,
		"request_failed": baselineFailed, "invalid_response": baselineFailed, "io_error": baselineFailed,
	} {
		reset(t)
		enabled = true

		BaselineResolved("cloud", status, 0, 0)

		require.Len(t, events, 1, status)
		assert.Equal(t, id, events[0].Id, status)
		if id == baselineFound {
			assert.NotContains(t, events[0].Data, "reason")
		} else {
			assert.Equal(t, status, events[0].Data["reason"])
		}
		if id == baselineNone {
			assert.NotContains(t, events[0].Data, "source", "the reason tells it")
		}
	}
}

func TestScanMode(t *testing.T) {
	for runScenario, expected := range map[string]struct {
		mode     string
		scenario any
	}{
		"":               {scanFull, nil},
		"default":        {scanFull, nil},
		"scope":          {scanIncremental, "scope"},
		"reversed-scope": {scanIncremental, "reversed-scope"},
		"full-history":   {scanFullHistory, nil},
	} {
		reset(t)
		enabled = true

		StartAnalysis(runScenario)(1, "")

		assert.Equal(t, expected.mode, events[0].Data["scan_mode"], runScenario)
		assert.Equal(t, analysisFailed, events[1].Id)
		assert.Equal(t, expected.mode, events[1].Data["scan_mode"], runScenario)
		assert.Equal(t, expected.scenario, events[1].Data["scenario"], runScenario)
	}
}

func TestAnalysisFailsWithItsReasons(t *testing.T) {
	reset(t)
	enabled = true

	StartAnalysis("")(1, ReasonLinterError)
	// a Docker engine without Linux, which docker.failed tells too
	StartAnalysis("")(1, ReasonDockerNotLinux)
	// a linter container hands over the plugins it can't install
	StartAnalysis("")(1, ReasonPluginsFailed)
	StartAnalysis("")(1, ReasonCancelled)

	assert.Equal(t, ReasonLinterError, events[1].Data["reason"])
	assert.Equal(t, ReasonDockerNotLinux, events[3].Data["reason"])
	assert.Equal(t, ReasonPluginsFailed, events[5].Data["reason"])
	assert.Equal(t, other, events[7].Data["reason"], "qodana scan is cancelled before the analysis")
}

func TestMissingStartCommitIsTheScenario(t *testing.T) {
	reset(t)
	enabled = true

	finishAnalysis := StartAnalysis("scope")
	GitPrepared(false)
	finishAnalysis(0, "")

	assert.Equal(t, map[string]any{"start_commit": "missing"}, events[1].Data)
	assert.Equal(t, analysisSucceeded, events[2].Id)
	assert.Equal(t, ScenarioStartCommitMissing, events[2].Data["scenario"])
	assert.Equal(t, ReasonSuccess, events[2].Data["result"])
}

func TestConfigSource(t *testing.T) {
	for source, files := range map[string][2]bool{
		"none": {false, false}, "local": {true, false}, "global": {false, true}, "local_and_global": {true, true},
	} {
		reset(t)
		enabled = true

		ConfigResolved(files[0], files[1])

		assert.Equal(t, []event{{groupRun, configResolved, events[0].Time, map[string]any{"source": source}}}, events)
	}
}

func TestFailedStepsAreTheReason(t *testing.T) {
	for id, failure := range map[string]struct {
		fail   func()
		reason string
	}{configFailed: {ConfigFailed, ReasonInvalidConfig}, gitFailed: {GitFailed, ReasonGitMissing}} {
		received := fakeFus(t)
		Start("scan", func() (string, string) { return testDevice, "" }, false, nil)
		failure.fail()
		// the log.Fatal after it
		Finish(1, ReasonFatal)

		report := <-received
		assert.Empty(t, lastEvent(report, id), "the event tells the reason")
		assert.Equal(t, failure.reason, lastEvent(report, commandFailed)["reason"], id)
		assert.Equal(t, id, lastEvent(report, commandFailed)["stage"])
	}
}

func TestBootstrap(t *testing.T) {
	reset(t)
	enabled = true

	BootstrapFinished(0, 2*time.Second)
	// the second commit of a diff
	BootstrapFinished(0, time.Second)
	require.Len(t, events, 1)
	assert.Equal(t, map[string]any{"duration_ms": int64(2000)}, events[0].Data)

	BootstrapFinished(3, time.Second)
	assert.Equal(
		t, map[string]any{"exit_code": 3, "duration_ms": int64(1000)}, events[1].Data,
	)
	assert.Equal(t, ReasonBootstrapFailed, knownReason)
}

func TestLinterNameHasNoMode(t *testing.T) {
	reset(t)
	enabled = true

	LinterSelected("QDJVM", true, "")
	LinterSelected("QDJVM", true, "docker")

	assert.Equal(t, map[string]any{"linter": "QDJVM", "preconfigured": true}, events[0].Data, "qodana scan chooses it")
	assert.Equal(t, "docker", events[1].Data["mode"])
}

func TestFailureIsRecordedOnce(t *testing.T) {
	reset(t)
	enabled = true

	RejectLicense("token_invalid")
	RejectLicense("token_invalid")

	require.Len(t, events, 1)
	assert.Equal(t, ReasonLicenseRejected, knownReason)
}

func TestContainerFailureIsTheStage(t *testing.T) {
	cacheDir := t.TempDir()
	reset(t)
	enabled = true
	track(groupRun, analysisStarted, nil)
	handOver(
		filepath.Join(cacheDir, handoverName), handover{
			Events: []event{{groupRun, gitFailed, 1, map[string]any{"reason": ReasonGitMissing}}},
			Reason: ReasonGitMissing,
		},
	)

	CollectFromContainer(cacheDir)
	track(groupRun, analysisFailed, nil)

	assert.Equal(t, gitFailed, cmp.Or(failedStage, stage))
	assert.Equal(t, ReasonGitMissing, exitReason(1, ""))
}

func TestFinishClosesTheAnalysis(t *testing.T) {
	received := fakeFus(t)
	Start("scan", func() (string, string) { return testDevice, "" }, false, nil)
	StartAnalysis("")
	GitFailed()
	// the log.Fatal after it, which leaves the analysis open
	Finish(1, ReasonFatal)

	report := <-received
	assert.Equal(t, ReasonGitMissing, lastEvent(report, analysisFailed)["reason"], "the reason of command.failed")
	assert.Equal(t, gitFailed, lastEvent(report, commandFailed)["stage"], "the step it failed at")
}

func TestContainerTellsHowItsAnalysisEnded(t *testing.T) {
	cacheDir := t.TempDir()
	reset(t)
	enabled = true
	finishAnalysis := StartAnalysis("")
	handOver(
		filepath.Join(cacheDir, handoverName), handover{
			Events:         []event{{groupRun, publishFailed, 1, nil}},
			Reason:         ReasonUploadFailed,
			AnalysisReason: ReasonSuccess,
		},
	)

	CollectFromContainer(cacheDir)
	finishAnalysis(1, "")
	// the host tells another result: the one of the container is kept
	ReportPublished(true, true)

	assert.Equal(t, []string{analysisStarted, publishFailed, analysisSucceeded}, ids(events), "one publish event")
	assert.Equal(t, ReasonUploadFailed, exitReason(1, ""), "the reason of command.failed")
	assert.Equal(t, publishFailed, cmp.Or(failedStage, stage), "as in a native run")
}

func TestAnalysisTellsTheLinter(t *testing.T) {
	reset(t)
	enabled = true

	LinterReady("QDCPP", "2025.3", true, false, "third_party")
	StartAnalysis("")(0, "")

	assert.Equal(t, "QDCPP", events[2].Data["linter"])
	assert.Equal(t, "third_party", events[2].Data["mode"])
}

func TestQodanaImageIsTheContainerMode(t *testing.T) {
	reset(t)
	enabled = true
	t.Setenv(qdenv.QodanaDockerEnv, "true")

	LinterReady("QDJVM", "2025.3", true, false, "native")
	StartAnalysis("")(0, "")

	assert.Equal(t, SetupNativePreinstalled, events[0].Data["setup"])
	assert.Equal(t, "container", events[2].Data["mode"])
}

func TestOlderContainerWithoutTheAnalysisReason(t *testing.T) {
	cacheDir := t.TempDir()
	reset(t)
	enabled = true
	finishAnalysis := StartAnalysis("")
	handOver(filepath.Join(cacheDir, handoverName), handover{Reason: ReasonLinterError})

	CollectFromContainer(cacheDir)
	finishAnalysis(1, "")

	assert.Equal(t, ReasonLinterError, events[1].Data["reason"])
}

func TestStageIsTheFailedStep(t *testing.T) {
	received := fakeFus(t)
	Start("scan", func() (string, string) { return testDevice, "" }, false, nil)
	StartAnalysis("")
	BootstrapFinished(2, time.Second)
	GitPrepared(true)
	Finish(2, "")

	assert.Equal(t, bootstrapFailed, lastEvent(<-received, commandFailed)["stage"], "not a later event")
}

func ids(events []event) []string {
	result := make([]string, 0, len(events))
	for _, e := range events {
		result = append(result, e.Id)
	}
	return result
}

func TestAnalysisEndsOnce(t *testing.T) {
	reset(t)
	enabled = true

	StartAnalysis("")(1, ReasonLinterError)
	finishAnalysis(1, ReasonFatal)

	ends := slices.DeleteFunc(slices.Clone(events), func(e event) bool { return e.Id == analysisStarted })
	require.Len(t, ends, 1)
	assert.Equal(t, ReasonLinterError, ends[0].Data["reason"])
}

func TestReportPublished(t *testing.T) {
	for name, tc := range map[string]struct {
		hasToken, uploaded bool
		events             []string
	}{
		"uploaded":         {true, true, []string{reportPublished}},
		"failed to upload": {true, false, []string{publishFailed}},
		"no token":         {false, false, []string{}},
	} {
		reset(t)
		enabled = true

		ReportPublished(tc.hasToken, tc.uploaded)

		assert.Equal(t, tc.events, ids(events), name)
	}
}

func TestCommandIsOfTheStartAndTheFinish(t *testing.T) {
	received := fakeFus(t)
	Start("scan", func() (string, string) { return testDevice, "" }, false, nil)
	LicenseObtained("cloud", "ULTIMATE")
	Finish(0, "")

	report := <-received
	assert.NotNil(t, lastEvent(report, "scan"+startedSuffix), "the event tells the command")
	assert.NotContains(t, lastEvent(report, "scan"+startedSuffix), "command")
	assert.Equal(t, "scan", lastEvent(report, commandSucceeded)["command"])
	assert.NotContains(t, lastEvent(report, licenseObtained), "command")
}

func TestContainerOfAnotherHandoverVersion(t *testing.T) {
	cacheDir := t.TempDir()
	reset(t)
	enabled = true
	content, err := json.Marshal(handover{Version: handoverVersion + 1, Reason: ReasonLinterError})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, handoverName), content, 0o644))

	CollectFromContainer(cacheDir)

	assert.Empty(t, knownReason, "an unknown format is not read")
}

func TestLicenseOnlyContainerIsNotSent(t *testing.T) {
	for env, sent := range map[string]bool{
		qdenv.QodanaLicenseOnlyToken + "=token":                             false,
		qdenv.QodanaLicenseOnlyToken + "=token," + qdenv.QodanaToken + "=t": true,
		qdenv.QodanaToken + "=token":                                        true,
		qdenv.QodanaLicenseOnlyToken + "=":                                  true,
	} {
		reset(t)

		AllowContainer(strings.Split(env, ","))

		assert.Equal(t, sent, allowed, env)
	}
}

func TestPowerOfTwo(t *testing.T) {
	for value, expected := range map[uint64]string{
		0:   "1",
		1:   "1",
		3:   "2",
		6:   "4",
		12:  "8",
		63:  "32",
		64:  "64+",
		512: "64+",
	} {
		assert.Equal(t, expected, powerOfTwo(value, cpuCounts), value)
	}
	for value, expected := range map[uint64]string{0: "0", 1: "1", 15: "8", 127: "64", 256: "128+"} {
		assert.Equal(t, expected, powerOfTwo(value, memorySizes), value)
	}
}

func TestCiName(t *testing.T) {
	t.Setenv("CODEBUILD_BUILD_ID", "build:1")
	assert.Equal(t, "aws-codebuild", ciName())
	t.Setenv("TEAMCITY_VERSION", "2025.07")
	assert.Equal(t, "teamcity", ciName())
}

func TestUnknownCiIsOther(t *testing.T) {
	if os.Getenv("TEAMCITY_VERSION") != "" || cienvironment.DetectCIEnvironment() != nil {
		t.Skip("runs on a known CI")
	}
	t.Setenv("CI", "false")
	assert.Equal(t, "none", ciName())
	t.Setenv("CI", "true")
	assert.Equal(t, "other", ciName())
}

func TestBucket(t *testing.T) {
	for count, expected := range map[int]string{
		0:     "0",
		1:     "1+",
		9:     "1+",
		10:    "10+",
		999:   "100+",
		12345: "10000+",
		1e7:   "100000+",
	} {
		assert.Equal(t, expected, bucket(count), count)
	}
}

func TestDockerVersionOfDistributionBuild(t *testing.T) {
	reset(t)
	enabled = true

	DockerReady(4, 8*gb, "20.10.24+dfsg1")
	DockerReady(4, 8*gb, "26.1.4-ce")

	require.Len(t, events, 1, "the engine is reported once")
	assert.Equal(t, "20.10.24", events[0].Data["version"])
}

func TestContainerEnv(t *testing.T) {
	reset(t)
	assert.Equal(t, "DO_NOT_TRACK=1", ContainerEnv("/data/cache"))

	enabled = true
	assert.Equal(t, "QODANA_STATISTICS_FILE=/data/cache/"+handoverName, ContainerEnv("/data/cache"))
}

func TestExitReason(t *testing.T) {
	reset(t)
	assert.Equal(t, ReasonSetupError, exitReason(1, ""), "a failure before the analysis is no linter error")
	analyzing = true
	assert.Equal(t, ReasonLinterError, exitReason(1, ""))
	assert.Equal(t, ReasonFailThreshold, exitReason(255, ""))
	assert.Equal(t, ReasonFatal, exitReason(1, ReasonFatal))
	Fail(ReasonDockerUnavailable)
	assert.Equal(t, ReasonDockerUnavailable, exitReason(1, ReasonFatal), "a known failure is more precise")

	assert.True(t, isSuccess(ReasonEmptyChangeset))
	assert.False(t, isSuccess(ReasonFatal))
}

func TestRunIsSent(t *testing.T) {
	received := fakeFus(t)
	connected, fake := make(chan string, 1), connect
	connect = func(device string, build string) *sender {
		connected <- device + " " + build
		return fake(device, build)
	}
	SetVersion("2026.2.3")
	Start("scan", func() (string, string) { return testDevice, "" }, false, nil)
	SetCloudIds(testProject, testOrg)
	LicenseObtained("cloud", "ULTIMATE")
	BaselineResolved("cloud", "found", 0, time.Second)
	finishAnalysis := StartAnalysis("")
	finishAnalysis(0, "")
	Finish(0, "")

	report := <-received
	ids := make([]string, 0, len(report.Events))
	for _, e := range report.Events {
		ids = append(ids, e.Event.ID)
	}
	assert.Subset(
		t,
		ids,
		[]string{
			"scan" + startedSuffix, licenseObtained, baselineFound, analysisStarted, analysisSucceeded, commandSucceeded,
			"system",
		},
	)
	last := lastEvent(report, commandSucceeded)
	assert.Equal(t, ReasonSuccess, last["result"])
	assert.Equal(t, analysisSucceeded, last["stage"])
	assert.Equal(t, testProject, last["project_id"], "the ids of Qodana Cloud are sent as they are")
	assert.Equal(t, testOrg, last["org_id"])
	assert.Equal(t, testDevice+" 2026.2.3", <-connected, "the device id of the linter, the version of the binary")
}

func TestRejectedLicenseFinishesRun(t *testing.T) {
	received := fakeFus(t)
	Start("scan", func() (string, string) { return "", "" }, false, nil)

	SetTokenSource("prompt")
	RejectLicense("token_invalid")
	Finish(1, ReasonFatal)

	report := <-received
	last := lastEvent(report, commandFailed)
	assert.Equal(t, ReasonLicenseRejected, last["reason"], "the exit handler reports the rejection")
	assert.Equal(t, licenseRejected, last["stage"])
	assert.Equal(t, "prompt", lastEvent(report, licenseRejected)["token"])
}

func TestNothingIsCollectedWithoutPermission(t *testing.T) {
	licenseOnly := []string{qdenv.QodanaLicenseOnlyToken + "=token"}
	for name, run := range map[string]struct {
		setup        func(t *testing.T)
		env          []string
		noStatistics bool
	}{
		"license-only token":            {func(t *testing.T) { t.Setenv(qdenv.QodanaLicenseOnlyToken, "token") }, nil, false},
		"license-only token with --env": {func(*testing.T) {}, licenseOnly, false},
		"do not track":                  {func(t *testing.T) { t.Setenv(doNotTrackEnv, "1") }, nil, false},
		"no statistics or air-gapped":   {func(*testing.T) {}, nil, true},
	} {
		t.Run(
			name, func(t *testing.T) {
				fakeFus(t)
				connected := false
				connect = func(string, string) *sender { connected = true; return nil }
				t.Setenv(qdenv.QodanaToken, "")
				run.setup(t)

				gitRun := false
				Start("scan", func() (string, string) { gitRun = true; return "", "" }, run.noStatistics, run.env)
				Finish(0, "")

				assert.False(t, connected, "JetBrains should not be contacted")
				assert.False(t, gitRun, "the remote of the project should not be read")
				assert.Empty(t, events)
			},
		)
	}
}

func TestEnvOverridesTheTokensOfTheCli(t *testing.T) {
	t.Setenv(qdenv.QodanaLicenseOnlyToken, "token")
	t.Setenv(qdenv.QodanaToken, "")
	assert.True(t, isLicenseOnly(nil))
	assert.False(t, isLicenseOnly([]string{qdenv.QodanaToken + "=token"}), "the linter gets a Qodana Cloud token")
	t.Setenv(qdenv.QodanaLicenseOnlyToken, "")
	assert.False(t, isLicenseOnly(nil))
	assert.False(t, isLicenseOnly([]string{qdenv.QodanaLicenseOnlyToken + "="}))
}

func TestDisallowedRunIsNotSent(t *testing.T) {
	received := fakeFus(t)
	Start("scan", func() (string, string) { return "", "" }, false, nil)
	Allow(false)
	Allow(true)

	Finish(0, "")

	assert.Empty(t, received, "a license-only token is not allowed again")
}

func TestWithIds(t *testing.T) {
	reset(t)
	command, projectId = "scan", "project"

	result := withIds(
		[]event{
			{groupRun, "scan" + startedSuffix, 1, map[string]any{"ci": "none"}},
			{groupEnv, "system", 1, map[string]any{"os": "linux"}},
			{groupRun, commandFailed, 1, map[string]any{"reason": ReasonFatal}},
		},
	)

	assert.Equal(t, map[string]any{"project_id": "project", "ci": "none"}, result[0].Data, "the event tells the command")
	assert.Equal(t, "scan", result[2].Data["command"])
	assert.Equal(t, map[string]any{"os": "linux"}, result[1].Data, "environment events are not linked to the run")
}

func TestRunEventsHaveTheAnalysisId(t *testing.T) {
	reset(t)
	command = "scan"
	SetAnalysisId("0f8fad5b-d9cb-469f-a165-70867728950e")

	result := withIds([]event{{groupRun, "scan" + startedSuffix, 1, nil}})

	assert.Equal(t, map[string]any{"analysis_id": "0f8fad5b-d9cb-469f-a165-70867728950e"}, result[0].Data)
}

func TestAnalysisIdIsOnlyAUuid(t *testing.T) {
	for _, id := range []string{
		"", "build-42", "0F8FAD5B-D9CB-469F-A165-70867728950E", "https://ci.example.com/0f8fad5b-d9cb-469f-a165-70867728950e",
	} {
		assert.Empty(t, AnalysisId(id), id)
	}
}

func TestDisabledInTestsOfOtherPackages(t *testing.T) {
	reset(t)
	Start("scan", func() (string, string) { return "", "" }, false, nil)
	LicenseObtained("env", "")
	Finish(0, "")

	assert.Empty(t, events)
}

// fakeFus turns statistics on with a sender which delivers reports to the returned channel.
func fakeFus(t *testing.T) chan fus.Report {
	reset(t)
	disabled = false
	// the CI runs the tests with a license-only token, which turns statistics off
	t.Setenv(qdenv.QodanaLicenseOnlyToken, "")
	t.Setenv(qdenv.QodanaToken, "")
	received := make(chan fus.Report, 1)
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				var report fus.Report
				body, _ := io.ReadAll(r.Body)
				assert.NoError(t, json.Unmarshal(body, &report))
				received <- report
			},
		),
	)
	t.Cleanup(server.Close)
	connect = func(string, string) *sender {
		return &sender{client: fus.NewClient(server.URL, sendTimeout, "test"), validator: newTestValidator(t)}
	}
	return received
}

func lastEvent(report fus.Report, id string) map[string]any {
	for i := len(report.Events) - 1; i >= 0; i-- {
		if report.Events[i].Event.ID == id {
			return report.Events[i].Event.Data
		}
	}
	return nil
}

func reset(t *testing.T) {
	restore := func() {
		disabled, connect, build = true, newSender, version.Version
		enabled, collectionStarted, allowed, analyzing = false, false, true, false
		command, projectId, orgId, analysisId, scenario, stage, failedStage, knownReason, tokenSource = "", "", "", "", "", "", "", "", ""
		baselineKnown, hasBaseline, analysisMode, analysisDone, analysisReason = false, false, "", false, ""
		events, senders, collected, startedAt = nil, nil, nil, time.Time{}
		linter, setup = nil, nil
		finishOnce = sync.Once{}
	}
	t.Cleanup(restore)
	restore()
}

func TestRepositoryDeviceIsTheOneOfTheLinter(t *testing.T) {
	ids, bucket, err := deviceIdentity("200820300000000-9bfa-5df8-1c66-3a9e6438608a", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"device": "200820300000000-9bfa-5df8-1c66-3a9e6438608a"}, ids)
	// the bucket the IDE computes for this device id
	assert.Equal(t, 154, bucket)
}

func TestMachineDeviceWithoutRepositoryDevice(t *testing.T) {
	dataDir := t.TempDir()
	for _, device := range []string{"", noRepositoryDevice, "custom-device"} {
		ids, _, err := deviceIdentity(device, dataDir)
		require.NoError(t, err)
		assert.Regexp(t, `^[0-9a-f]{64}#C$`, ids["device"], device)
	}
}

func TestIdeBucket(t *testing.T) {
	// "polygenelubricants".hashCode() is Integer.MIN_VALUE, which nonNegativeAbs makes Integer.MAX_VALUE
	assert.Equal(t, 255, ideBucket("polygenelubricants"))
	assert.Equal(t, 0, ideBucket(""))
}

func TestOnlyReleasesAreSent(t *testing.T) {
	for _, release := range []string{"2026.2", "2026.2.3"} {
		assert.Regexp(t, releaseVersion, release)
	}
	// development, TeamCity snapshot, GitHub snapshot, nightly and TeamCity build number builds
	for _, internal := range []string{"dev", "1234", "2026.2.4-next", "2027.1-nightly", "2026.2.3.45"} {
		s, err := createSender(testDevice, internal)
		assert.Nil(t, s, internal)
		assert.ErrorContains(t, err, "is not a release", internal)
	}
}

func TestReleaseIsSentToFusAndPostHog(t *testing.T) {
	cacheDir := t.TempDir()
	for _, env := range []string{"HOME", "XDG_CACHE_HOME", "LocalAppData"} {
		t.Setenv(env, cacheDir)
	}
	userAgents := make(chan string, 2)
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { userAgents <- r.URL.Path + " " + r.UserAgent() }),
	)
	defer server.Close()
	host := postHogHost
	postHogHost = server.URL
	t.Cleanup(func() { postHogHost = host })
	// a fresh cached config is not fetched
	userCacheDir, err := os.UserCacheDir()
	require.NoError(t, err)
	config, err := json.Marshal(fus.FUSConfig{SendEndpoint: server.URL + "/fus", FetchedAt: time.Now().Unix()})
	require.NoError(t, err)
	dataDir := filepath.Join(userCacheDir, "JetBrains", "Qodana", "fus")
	require.NoError(t, os.MkdirAll(dataDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "fus_config.json"), config, 0o600))

	s := newSender(testDevice, "2026.2")
	require.NotNil(t, s)
	require.NoError(t, s.send([]event{{groupRun, licenseObtained, time.Now().UnixMilli(), map[string]any{"kind": "env"}}}))

	assert.ElementsMatch(t, []string{"/fus QDCLI/2026.2", "/batch/ QDCLI/2026.2"}, []string{<-userAgents, <-userAgents})
}

func TestPanicOfTheSenderSendsNothing(t *testing.T) {
	received := fakeFus(t)
	connect = func(string, string) *sender { panic("boom") }
	Start("scan", func() (string, string) { return "", "" }, false, nil)

	Finish(0, "")

	assert.Empty(t, received)
}

func TestJetBrainsRunsAreNotSent(t *testing.T) {
	for name, run := range map[string]struct {
		remoteUrl, endpoint  string
		internalOrganization bool
	}{
		"repository":                  {remoteUrl: "git@github.com:JetBrains/qodana-cli.git"},
		"repository of JetBrains Git": {remoteUrl: "ssh://git@git.jetbrains.team/qodana/cli.git"},
		"Qodana Cloud":                {endpoint: "https://qodana.labs.jb.gg"},
		"Qodana Cloud without scheme": {endpoint: "qodana.labs.jb.gg"},
		"internal organization":       {internalOrganization: true},
	} {
		t.Run(
			name, func(t *testing.T) {
				received := fakeFus(t)
				env := []string{qdenv.QodanaEndpointEnv + "=" + run.endpoint}
				Start("scan", func() (string, string) { return testDevice, run.remoteUrl }, false, env)
				SetCloudIds(testProject, testOrg)
				Allow(!run.internalOrganization)

				Finish(0, "")

				assert.Empty(t, received)
				assert.False(t, Allowed(), "the events of third-party linters are not sent either")
			},
		)
	}
}

func TestJetBrainsRunInAContainerIsNotAllowed(t *testing.T) {
	fakeFus(t)
	t.Setenv(handoverEnv, filepath.Join(t.TempDir(), "handover.json"))
	Start("scan", func() (string, string) { return testDevice, "git@github.com:JetBrains/qodana-cli.git" }, false, nil)

	assert.False(t, Allowed(), "a third-party linter sends its legacy events from the container")
}

func TestNotStartedRunIsNotAllowed(t *testing.T) {
	reset(t)
	disabled = false
	Start("scan", func() (string, string) { return testDevice, "" }, true, nil)

	assert.False(t, Allowed(), "an opted-out run sends no legacy events")
}

func TestCustomerRunIsSent(t *testing.T) {
	received := fakeFus(t)
	env := []string{qdenv.QodanaEndpointEnv + "=https://qodana.cloud"}
	Start("scan", func() (string, string) { return testDevice, "git@github.com:acme/app.git" }, false, env)
	SetCloudIds(testProject, testOrg)
	Allow(true)

	assert.True(t, Allowed())
	Finish(0, "")
	assert.NotNil(t, lastEvent(<-received, commandSucceeded))
}

func TestQodanaCloudOfTheCliIsChecked(t *testing.T) {
	received := fakeFus(t)
	t.Setenv(qdenv.QodanaEndpointEnv, "https://qodana.jetbrains.team")
	Start("scan", func() (string, string) { return testDevice, "" }, false, nil)
	Finish(0, "")

	assert.Empty(t, received)
}

func TestQodanaCloudOfTheLinterOverridesTheOneOfTheCli(t *testing.T) {
	fakeFus(t)
	t.Setenv(qdenv.QodanaEndpointEnv, "https://qodana.jetbrains.team")
	env := []string{qdenv.QodanaEndpointEnv + "=https://qodana.acme.com"}
	Start("scan", func() (string, string) { return testDevice, "" }, false, env)

	assert.True(t, Allowed(), "the linter reports to the Qodana Cloud of --env")
}

func TestEnvValue(t *testing.T) {
	key := qdenv.QodanaEndpointEnv
	t.Setenv(key, "cli")
	assert.Equal(t, "cli", envValue(nil, key))
	assert.Equal(t, "cli", envValue([]string{"OTHER=value"}, key))
	assert.Equal(t, "", envValue([]string{key}, key), "Docker removes a variable without a value from the container")
	assert.Equal(t, "b=c", envValue([]string{key + "=a", key + "=b=c"}, key), "the last one wins and keeps its =")
}

func TestJetBrainsHost(t *testing.T) {
	for host, expected := range map[string]bool{
		"jetbrains.com":            true,
		"qodana.jetbrains.com":     true,
		"git.jetbrains.team":       true,
		"buildserver.intellij.net": true,
		"qodana.labs.jb.gg":        true,
		"notjetbrains.com":         false,
		"jetbrains.com.acme.com":   false,
		"jetbrains.space":          false,
		"acme.jetbrains.space":     false,
		"qodana.cloud":             false,
		"":                         false,
	} {
		assert.Equal(t, expected, isJetBrainsHost(host), host)
	}
}

func TestParseUrl(t *testing.T) {
	for rawUrl, expected := range map[string][2]string{
		"https://github.com/JetBrains/qodana-cli.git": {"github.com", "JetBrains"},
		"https://user:pass@GitHub.com:443/Acme/app":   {"github.com", "Acme"},
		"git@github.com:JetBrains/qodana-cli.git":     {"github.com", "JetBrains"},
		"ssh://git@git.jetbrains.team:22/qodana/cli":  {"git.jetbrains.team", "qodana"},
		"github.com/acme/app":                         {"github.com", "acme"},
		"qodana.labs.jb.gg":                           {"qodana.labs.jb.gg", ""},
		"https://qodana.cloud/":                       {"qodana.cloud", ""},
		"/srv/git/jetbrains/app.git":                  {"", "srv"},
		"file:///srv/git/JetBrains/app.git":           {"", "srv"},
		"":                                            {"", ""},
	} {
		host, owner := parseUrl(rawUrl)
		assert.Equal(t, expected, [2]string{host, owner}, rawUrl)
	}
}

func TestJetBrainsRepository(t *testing.T) {
	for remoteUrl, expected := range map[string]bool{
		"https://github.com/JetBrains/qodana-cli.git":           true,
		"git@github.com:JetBrains-Research/code.git":            true,
		"github.com/jetbrains/kotlin":                           true,
		"ssh://git@git.jetbrains.team/qodana/cli.git":           true,
		"https://code.jetbrains.team/p/qodana/repositories/cli": true,
		"https://github.com/acme/jetbrains-plugin.git":          false,
		"https://git.jetbrains.space/acme/project/app.git":      false,
		"git@gitlab.com:acme/app.git":                           false,
		"git@bitbucket.org:JetBrains/app.git":                   true,
		"https://GitHub.com/JetBrains/app.git":                  true,
		"https://gitlab.acme.com/jetbrains-tools/app.git":       false,
		"git@github.com:JetBrainsFan/app.git":                   false,
		"/srv/git/JetBrains/app.git":                            false,
		"https://bitbucket.org/jetbrains-research/app":          true,
		"": false,
	} {
		assert.Equal(t, expected, isJetBrainsRepository(remoteUrl), remoteUrl)
	}
}
