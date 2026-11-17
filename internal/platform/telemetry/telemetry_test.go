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
	ids := map[string]any{"command": "scan", "project_id": testProject, "org_id": testOrg}
	system, software := environment()
	testEvents := []event{
		{groupRun, started, 0, map[string]any{"ci": "github-actions", "container": false, "interactive": true}},
		{groupRun, licenseObtained, 0, map[string]any{"kind": "cloud", "plan": "ULTIMATE_PLUS"}},
		{groupRun, licenseRejected, 0, map[string]any{"reject_reason": "token_declined"}},
		{groupRun, linterReady, 0, map[string]any{"linter": "QDJVM", "version": "2025.3", "paid": true, "eap": false, "mode": "native"}},
		{groupRun, linterReady, 0, map[string]any{"linter": "QDPYC", "version": "latest", "paid": false, "eap": true, "mode": "docker"}},
		{groupRun, baselineResolved, 0, map[string]any{"source": "cloud", "baseline_status": "found", "request_ms": int64(250)}},
		{groupRun, baselineResolved, 0, map[string]any{"source": "cloud", "baseline_status": "request_failed", "http_status": 403, "request_ms": int64(25)}},
		{groupRun, analysisStarted, 0, map[string]any{}},
		{groupRun, analysisFinished, 0, map[string]any{"exit_code": 255, "reason": ReasonLinterError, "duration_ms": int64(1234), "scenario": "reversed-scope"}},
		{groupRun, linterSelected, 0, map[string]any{"linter": "QDNETC", "preconfigured": false}},
		{groupRun, linterDownloaded, 0, map[string]any{"mode": "docker", "cached": false, "download_ms": int64(42000)}},
		{groupRun, licenseRejected, 0, map[string]any{"reject_reason": "expired", "token": "keychain"}},
		{groupRun, problemsFound, 0, map[string]any{"total": "100+", "new": "10+", "unchanged": "100+", "absent": "0", "baseline": "applied"}},
		{groupRun, reportPublished, 0, map[string]any{"publish_status": "uploaded"}},
		{groupRun, finished, 0, map[string]any{"exit_code": -1, "reason": ReasonImagePull, "success": false, "stage": problemsFound, "duration_ms": int64(5)}},
		{groupEnv, "system", 0, system},
		{groupEnv, "software", 0, software},
		{groupEnv, dockerEngine, 0, map[string]any{"cpu_count": "8", "ram_gb": "4", "version": "27.3.1"}},
	}
	for _, e := range testEvents {
		t.Run(e.Group+"/"+e.Id, func(t *testing.T) {
			data := maps.Clone(e.Data)
			if e.Group == groupRun {
				maps.Copy(data, ids)
			}
			assert.LessOrEqual(t, len(data), fus.MaxDataFields)
			validated, dropped := validator.Validate(fus.LogEvent{Group: fus.Group(e.Group, 1), Event: fus.EventAction{ID: e.Id, Data: data}})
			require.False(t, dropped)
			assert.Equal(t, e.Id, validated.Event.ID)
			for key, value := range validated.Event.Data {
				if s, ok := value.(string); ok {
					assert.False(t, strings.HasPrefix(s, "validation.") || s == "undefined", "%s=%v is rejected", key, e.Data[key])
				}
			}
		})
	}
}

// ids as the linter and Qodana Cloud have them
const (
	testDevice  = "200820300000000-9bfa-5df8-1c66-3a9e6438608a"
	testProject = "5e3c9a1b-2f7d-3c44-8a1e-9b0d6f2c4e71"
	testOrg     = "a3f09c2e71b4d856"
)

func TestSendKeepsEventTimeAndIds(t *testing.T) {
	var report fus.Report
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		assert.NoError(t, json.Unmarshal(body, &report))
	}))
	defer server.Close()
	s := &sender{
		client:    fus.NewClient(server.URL, sendTimeout, "test"),
		validator: newTestValidator(t),
		base:      fus.LogEvent{Session: "session"},
	}

	err := s.send([]event{{groupRun, analysisStarted, 1700000000000, map[string]any{"command": "scan", "project_id": testProject}}})

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/batch/", r.URL.Path)
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&batch))
	}))
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

	err := s.send([]event{
		{groupRun, licenseObtained, 1700000000000, map[string]any{"command": "scan", "kind": "cloud", "plan": "ULTIMATE", "project_id": testProject, "org_id": testOrg}},
		{groupRun, licenseRejected, 1700000000001, map[string]any{"command": "scan", "reject_reason": "invented", "org_id": "org hash"}},
		{groupEnv, "system", 1700000000002, map[string]any{"os": "linux"}},
		{groupEnv, dockerEngine, 1700000000003, map[string]any{"version": "27.3.1"}},
	})

	require.NoError(t, err)
	assert.Equal(t, "phc_test", batch.ApiKey)
	require.Len(t, batch.Batch, 4)
	license := batch.Batch[0]
	assert.Equal(t, "qd.cli.run.license.obtained", license.Event)
	assert.Equal(t, testDevice, license.DistinctId, "the subject is the device, stable per repository")
	assert.Equal(t, false, license.Properties["$process_person_profile"])
	assert.Equal(t, time.UnixMilli(1700000000000).UTC(), license.Timestamp)
	assert.Equal(t, map[string]any{"organization": testOrg, "project": testProject, "repository": testDevice}, license.Properties["$groups"])
	assert.Equal(t, "ULTIMATE", license.Properties["plan"])
	assert.Equal(t, "session", license.Properties["session"])
	rejected := batch.Batch[1]
	assert.NotEqual(t, "invented", rejected.Properties["reject_reason"], "PostHog gets the values FUS accepts")
	assert.NotContains(t, rejected.Properties, "org_id", "a rejected id is no id")
	assert.Equal(t, map[string]any{"repository": testDevice}, rejected.Properties["$groups"])
	assert.Equal(t, testDevice, batch.Batch[2].DistinctId, "environment events are of the device of the run")
	assert.Equal(t, "2026.1", batch.Batch[3].Properties["cli_version"])
	assert.Equal(t, "27.3.1", batch.Batch[3].Properties["version"], "the CLI version doesn't replace the one of the event")
}

func TestContainerHandsTheRunOver(t *testing.T) {
	cacheDir := t.TempDir()
	// the CLI in the container
	fakeFus(t)
	t.Setenv(handoverEnv, filepath.Join(cacheDir, handoverName))
	Start("scan", func() string { return testDevice }, false, nil)
	LicenseObtained("cloud", "ULTIMATE")
	SetScenario(ScenarioStartCommitMissing)
	Allow(false)
	Finish(1, ReasonLinterError)
	// the CLI on the host
	reset(t)
	enabled, scenario = true, "scope"

	CollectFromContainer(cacheDir)

	assert.Equal(t, ScenarioStartCommitMissing, scenario)
	require.Len(t, events, 1)
	assert.Equal(t, licenseObtained, events[0].Id)
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
			handOver(filepath.Join(cacheDir, handoverName), handover{Allowed: true, StartCommitMissing: missing})
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
	handOver(file, handover{
		Events: []event{
			{groupRun, started, 1, nil},
			{groupRun, licenseRejected, 2, map[string]any{"reject_reason": "token_declined"}},
		},
		ProjectId: "project",
		Reason:    ReasonLicenseRejected,
		Allowed:   true,
	})
	reset(t)
	enabled = true

	CollectFromContainer(cacheDir)

	assert.Equal(t, []event{{groupRun, licenseRejected, 2, map[string]any{"reject_reason": "token_declined"}}}, events)
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
		handOver(filepath.Join(cacheDir, handoverName), handover{Events: []event{license}, Reason: reason, Allowed: true})
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
	handOver(filepath.Join(cacheDir, handoverName), handover{
		Events: []event{{groupRun, licenseRejected, 1, map[string]any{"reject_reason": "token_declined"}}},
		Reason: ReasonLicenseRejected,
	})

	CollectFromContainer(cacheDir)
	track(groupRun, analysisFinished, nil)
	Finish(1, "")

	assert.Equal(t, licenseRejected, cmp.Or(failedStage, stage))
}

func TestExpiredLicenseIsRejected(t *testing.T) {
	reset(t)
	enabled = true

	StartAnalysis()(exitcodes.QodanaEapLicenseExpiredExitCode, "")

	assert.True(t, slices.ContainsFunc(events, func(e event) bool {
		return e.Id == licenseRejected && e.Data["reject_reason"] == "expired"
	}))
}

func TestGivenReasonKeepsTheKnownOne(t *testing.T) {
	reset(t)
	enabled = true

	StartAnalysis()(0, ReasonEmptyChangeset)
	Fail(ReasonLicenseRejected)
	StartAnalysis()(3, ReasonTimeout)

	assert.Equal(t, ReasonEmptyChangeset, events[1].Data["reason"])
	assert.Equal(t, ReasonLicenseRejected, events[3].Data["reason"], "a container may have handed a failure over")
}

func TestExpiredLicenseInContainerIsNotTheStage(t *testing.T) {
	cacheDir := t.TempDir()
	reset(t)
	enabled = true
	track(groupRun, analysisStarted, nil)
	handOver(filepath.Join(cacheDir, handoverName), handover{
		Events: []event{{groupRun, licenseRejected, 1, map[string]any{"reject_reason": "expired"}}},
		Reason: ReasonLicenseExpired,
	})

	CollectFromContainer(cacheDir)
	track(groupRun, analysisFinished, nil)

	assert.Equal(t, analysisFinished, cmp.Or(failedStage, stage), "as in a native run")
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
	assert.Equal(t, map[string]any{"total": "10+", "new": "1+", "unchanged": "10+", "absent": "100+", "baseline": "applied"},
		events[2].Data, "absent problems are not of the project")
	assert.Equal(t, "unknown", events[3].Data["baseline"], "no problems do not tell")
	assert.Equal(t, "ignored", events[4].Data["baseline"])
}

func TestTokenSourceIsTheFirstOne(t *testing.T) {
	reset(t)

	SetTokenSource("org_token")
	SetTokenSource("env")

	assert.Equal(t, "org_token", tokenSource, "the exchanged token is in the environment afterwards")
}

func TestValuesOutsideEnumsAreOther(t *testing.T) {
	assert.Equal(t, "ULTIMATE", oneOf("ULTIMATE", globalRules.Enums[enumLicenseType]))
	assert.Equal(t, other, oneOf("STARTER", globalRules.Enums[enumLicenseType]))
	assert.Equal(t, "opensuse", distro("opensuse-leap"))
	assert.Equal(t, other, distro("gentoo"))
}

func TestVersionName(t *testing.T) {
	for version, expected := range map[string]string{"2025.1": "2025.1", "27.3.1": "27.3.1", "24": "24", "my": other, "": other, "1..2": other} {
		assert.Equal(t, expected, versionName(version), version)
	}
	reset(t)
	enabled = true

	LinterReady("QDJVM", "my-build", true, false, "docker")
	LinterReady("QDJVM", "latest", true, false, "docker")

	assert.Equal(t, other, events[0].Data["version"])
	assert.Equal(t, "latest", events[1].Data["version"])
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
	for value, expected := range map[uint64]string{0: "1", 1: "1", 3: "2", 6: "4", 12: "8", 63: "32", 64: "64+", 512: "64+"} {
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
	for count, expected := range map[int]string{0: "0", 1: "1+", 9: "1+", 10: "10+", 999: "100+", 12345: "10000+", 1e7: "100000+"} {
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
	Start("scan", func() string { return testDevice }, false, nil)
	SetCloudIds(testProject, testOrg)
	LicenseObtained("cloud", "ULTIMATE")
	BaselineResolved("cloud", "found", 0, time.Second)
	finishAnalysis := StartAnalysis()
	finishAnalysis(0, "")
	Finish(0, "")

	report := <-received
	ids := make([]string, 0, len(report.Events))
	for _, e := range report.Events {
		ids = append(ids, e.Event.ID)
	}
	assert.Subset(t, ids, []string{started, licenseObtained, baselineResolved, analysisStarted, analysisFinished, finished, "system"})
	last := lastEvent(report, finished)
	assert.Equal(t, ReasonSuccess, last["reason"])
	assert.Equal(t, analysisFinished, last["stage"])
	assert.Equal(t, testProject, last["project_id"], "the ids of Qodana Cloud are sent as they are")
	assert.Equal(t, testOrg, last["org_id"])
	assert.Equal(t, testDevice+" 2026.2.3", <-connected, "the device id of the linter, the version of the binary")
}

func TestRejectedLicenseFinishesRun(t *testing.T) {
	received := fakeFus(t)
	Start("scan", func() string { return "" }, false, nil)

	SetTokenSource("prompt")
	RejectLicense("token_invalid")
	Finish(1, ReasonFatal)

	report := <-received
	last := lastEvent(report, finished)
	assert.Equal(t, ReasonLicenseRejected, last["reason"], "the exit handler reports the rejection")
	assert.Equal(t, licenseRejected, last["stage"])
	assert.Equal(t, "prompt", lastEvent(report, licenseRejected)["token"])
}

func TestNothingIsCollectedWithoutPermission(t *testing.T) {
	licenseOnly := []string{qdenv.QodanaLicenseOnlyToken + "=token"}
	for name, run := range map[string]struct {
		setup func(t *testing.T)
		env   []string
	}{
		"license-only token":            {func(t *testing.T) { t.Setenv(qdenv.QodanaLicenseOnlyToken, "token") }, nil},
		"license-only token with --env": {func(*testing.T) {}, licenseOnly},
		"do not track":                  {func(t *testing.T) { t.Setenv(doNotTrackEnv, "1") }, nil},
	} {
		t.Run(name, func(t *testing.T) {
			fakeFus(t)
			connected := false
			connect = func(string, string) *sender { connected = true; return nil }
			t.Setenv(qdenv.QodanaToken, "")
			run.setup(t)

			Start("scan", func() string { return "" }, false, run.env)
			Finish(0, "")

			assert.False(t, connected, "JetBrains should not be contacted")
			assert.Empty(t, events)
		})
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
	Start("scan", func() string { return "" }, false, nil)
	Allow(false)
	Allow(true)

	Finish(0, "")

	assert.Empty(t, received, "a license-only token is not allowed again")
}

func TestWithIds(t *testing.T) {
	reset(t)
	command, projectId = "scan", "project"

	result := withIds([]event{{groupRun, started, 1, map[string]any{"ci": "none"}}, {groupEnv, "system", 1, map[string]any{"os": "linux"}}})

	assert.Equal(t, map[string]any{"command": "scan", "project_id": "project", "ci": "none"}, result[0].Data)
	assert.Equal(t, map[string]any{"os": "linux"}, result[1].Data, "environment events are not linked to the run")
}

func TestDisabledInTestsOfOtherPackages(t *testing.T) {
	reset(t)
	Start("scan", func() string { return "" }, false, nil)
	LicenseObtained("env", "")
	Finish(0, "")

	assert.Empty(t, events)
}

// fakeFus turns statistics on with a sender which delivers reports to the returned channel.
func fakeFus(t *testing.T) chan fus.Report {
	reset(t)
	disabled = false
	received := make(chan fus.Report, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var report fus.Report
		body, _ := io.ReadAll(r.Body)
		assert.NoError(t, json.Unmarshal(body, &report))
		received <- report
	}))
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
	clear := func() {
		disabled, connect, build = true, newSender, version.Version
		enabled, allowed, analyzing = false, true, false
		command, projectId, orgId, scenario, stage, failedStage, knownReason, tokenSource = "", "", "", "", "", "", "", ""
		baselineFound = false
		events, senders, collected, startedAt = nil, nil, nil, time.Time{}
		finishOnce = sync.Once{}
	}
	t.Cleanup(clear)
	clear()
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
