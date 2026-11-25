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
	fus "github.com/JetBrains/fus-reporting-api-go"
)

const (
	recorderID      = "QD"
	recorderVersion = 1
	productCode     = "QDCLI"
	// salt is the anonymization namespace of the machine device id; changing it breaks hash continuity.
	salt = "qodana-cli-fus-v1"
	// repositoryDevice is the device id the CLI gives the linter, of the repository: see platform.GetDeviceIdSalt.
	// The CLI sends it as the linter does, so a run of both is one device in FUS. noRepositoryDevice is the one of
	// every project without a remote, the CLI sends its hashed machine id then.
	repositoryDevice   = `200820300000000-[0-9a-f]+-[0-9a-f]+-[0-9a-f]+-[0-9a-f]+`
	noRepositoryDevice = "200820300000000-0000-0000-0000-000000000000"
	// projectIdPattern and orgIdPattern are of Qodana Cloud's ProjectIdHasher and OrganizationIdHasher, which give
	// [-0-9A-Fa-f]{32,64} and [0-9A-Za-z]{2,16}: the FUS rule syntax has no quantifiers to check the length with.
	projectIdPattern = `[-0-9A-Fa-f]+`
	orgIdPattern     = `[0-9A-Za-z]+`
	// analysisIdPattern is of the analysis id, a UUID the CLI gives the linter as qodana.automation.guid: see AnalysisId.
	analysisIdPattern = `[0-9a-f]+-[0-9a-f]+-[0-9a-f]+-[0-9a-f]+-[0-9a-f]+`

	groupRun     = "qd.cli.run"
	groupEnv     = "qd.cli.env"
	groupVersion = 1
)

// Events of the qd.cli.run funnel, in the order a successful native scan emits them. The linter and license steps
// are unordered across modes: a third-party linter checks the license before it is ready, and a linter container
// checks it during the analysis, so its license events are timestamped after analysis.started.
const (
	started          = "started"
	linterSelected   = "linter.selected"
	linterDownloaded = "linter.downloaded"
	linterReady      = "linter.ready"
	licenseObtained  = "license.obtained"
	licenseRejected  = "license.rejected"
	baselineResolved = "baseline.resolved"
	analysisStarted  = "analysis.started"
	analysisFinished = "analysis.finished"
	problemsFound    = "problems.found"
	reportPublished  = "report.published"
	finished         = "finished"
	// the steps of qodana show and qodana contributors
	reportShown         = "report.shown"
	contributorsCounted = "contributors.counted"
)

// ScenarioStartCommitMissing is the scenario of a diff run whose start commit is missing, so it analyzes the current
// state, as the default scenario does.
const ScenarioStartCommitMissing = "default-start-commit-missing"

// dockerEngine is the qd.cli.env event of the container engine, the other ones are tracked with literal ids.
const dockerEngine = "docker"

// Reasons for analysis.finished and finished events.
const (
	ReasonSuccess         = "success"
	ReasonFailThreshold   = "fail_threshold"
	ReasonEmptyChangeset  = "empty_changeset"
	ReasonLicenseExpired  = "license_expired"
	ReasonTimeout         = "timeout"
	ReasonOutOfMemory     = "out_of_memory"
	ReasonLinterError     = "linter_error"
	ReasonInternalError   = "internal_error"
	ReasonLicenseRejected = "license_rejected"
	ReasonNoFiles         = "no_files"
	ReasonCancelled       = "cancelled"
	ReasonInterrupted     = "interrupted"
	ReasonFatal           = "fatal"
	ReasonBaselineError   = "baseline_error"
	ReasonUploadFailed    = "upload_failed"
	ReasonReportFailed    = "report_failed"
	// qodana show finds no report, or can't serve it
	ReasonReportMissing = "report_missing"
	ReasonServeFailed   = "serve_failed"
	// failures before the analysis
	ReasonSetupError         = "setup_error"
	ReasonUnsupportedProject = "unsupported_project"
	ReasonInvalidConfig      = "invalid_config"
	ReasonLinterDownload     = "linter_download_failed"
	ReasonDockerUnavailable  = "docker_unavailable"
	ReasonImagePull          = "image_pull_failed"
	ReasonGitMissing         = "git_missing"
	ReasonBootstrapFailed    = "bootstrap_failed"
)

var reasons = []string{
	ReasonSuccess, ReasonFailThreshold, ReasonEmptyChangeset, ReasonLicenseExpired, ReasonTimeout,
	ReasonOutOfMemory, ReasonLinterError, ReasonInternalError, ReasonLicenseRejected, ReasonNoFiles,
	ReasonCancelled, ReasonInterrupted, ReasonFatal, ReasonBaselineError, ReasonUploadFailed,
	ReasonSetupError, ReasonUnsupportedProject, ReasonInvalidConfig, ReasonLinterDownload,
	ReasonDockerUnavailable, ReasonImagePull, ReasonGitMissing, ReasonReportFailed, ReasonBootstrapFailed,
	ReasonReportMissing, ReasonServeFailed,
}

// other is sent for a value outside its enum, which FUS would drop
const other = "other"

var (
	buckets = []string{"0", "1+", "10+", "100+", "1000+", "10000+", "100000+"}
	// ciNames are the CIs of ci-environment, as qdenv.GetCIName names them, and the ones only ciName detects
	ciNames = []string{
		"azure-pipelines", "bamboo", "bitrise", "buddy", "circleci", "codefresh", "codeship", "github-actions",
		"gitlab", "gocd", "jenkins", "semaphore", "travis-ci", "wercker", "teamcity", "space", "bitbucket",
		"aws-codebuild",
	}
	// shells are the ones of SHELL, which Windows doesn't set
	shells  = []string{"bash", "zsh", "sh", "dash", "fish", "ksh", "tcsh", "csh"}
	distros = []string{
		"ubuntu", "debian", "alpine", "centos", "redhat", "fedora", "amazon", "opensuse", "suse", "arch", "rocky",
		"almalinux", "oracle",
	}
	// linters are the product codes of product.AllLinters, which imports this package; a test compares them
	linters = []string{
		"QDJVMC", "QDJVM", "QDANDC", "QDAND", "QDPHP", "QDPYC", "QDPY", "QDJS", "QDNETC", "QDNET", "QDRUBY",
		"QDCPP", "QDGO", "QDRST", "QDCLC", "QDPOLY",
	}
	cpuCounts   = []string{"1", "2", "4", "8", "16", "32", "64+"}
	memorySizes = []string{"0", "1", "2", "4", "8", "16", "32", "64", "128+"}
)

// Global rules of the FUS metadata, the only rules a field refers to. The values are local copies for the
// validation before sending, the server validates with its own: a copy may accept less, never more.
const (
	enumBoolean     = "boolean"
	enumLicenseType = "qodana_license_type"
	regexpInteger   = "integer"
	regexpVersion   = "version"
)

var globalRules = &fus.SchemeRules{
	Enums: map[string][]string{
		enumBoolean: {"true", "false"},
		// as in resources.jetbrains.com/storage/ap/fus/metadata/tiger/QD/groups/QD.json
		enumLicenseType: {
			"EAP_COMMUNITY", "EAP_ULTIMATE", "EAP_ULTIMATE_PLUS", "TRIAL_COMMUNITY", "TRIAL_ULTIMATE",
			"TRIAL_ULTIMATE_PLUS", "COMMUNITY", "ULTIMATE", "ULTIMATE_PLUS", "PREMIUM", "PREMIUM_LITE",
		},
	},
	Regexps: map[string]string{
		regexpInteger: `\d+`,
		regexpVersion: `\d+(\.\d+)*`,
	},
}

var (
	integerRule = fus.RegexpRefExpr(regexpInteger)
	versionRule = fus.RegexpRefExpr(regexpVersion)
	// exitCodeRules accept -1, the exit code of a step the CLI fails itself
	exitCodeRules = []string{integerRule, fus.EnumExpr("-1")}
)

var definition = &fus.Definition{
	Version: "1",
	Rules:   globalRules,
	Groups: []fus.GroupDefinition{
		{
			ID: groupRun, Version: groupVersion, Description: "Qodana CLI command run funnel",
			Events: []fus.EventDefinition{
				runEvent(started, "Command started",
					boolField("container", "The CLI runs in a Qodana container"),
					boolField("interactive", "The CLI runs in an interactive terminal"),
					field("ci", "CI system the CLI runs on, none outside CI", fus.EnumExpr(append(ciNames, "none", other)...)),
				),
				runEvent(linterSelected, "Linter is selected for the project by qodana init",
					field("linter", "Product code of the linter", fus.EnumExpr(linters...)),
					boolField("preconfigured", "qodana.yaml of the project has the linter already"),
				),
				runEvent(linterDownloaded, "Linter is downloaded or its Docker image pulled, none with --skip-pull",
					field("mode", "Native linter or Docker image", fus.EnumExpr("native", "docker")),
					boolField("cached", "The linter or the image is up to date already"),
					field("download_ms", "Duration of the download or the pull", integerRule),
				),
				runEvent(licenseObtained, "License obtained",
					tokenField,
					field("kind", "Where the license comes from", fus.EnumExpr("env", "community", "eap", "cloud")),
					field("plan", "License plan of the Qodana Cloud organization",
						fus.EnumRefExpr(enumLicenseType), fus.EnumExpr(other)),
				),
				runEvent(licenseRejected, "License rejected",
					field("reject_reason", "Why the license is rejected", fus.EnumExpr(
						"no_token", "token_invalid", "token_declined", "request_failed", "community_plan", "empty_key", "expired",
					)),
					tokenField,
				),
				runEvent(linterReady, "Linter is installed, or its Docker image chosen, the image is pulled during the analysis",
					field("linter", "Product code of the linter", fus.EnumExpr(linters...)),
					field("version", "Version of the linter, latest for an image without a version", versionRule,
						fus.EnumExpr("latest", other)),
					boolField("paid", "The linter requires a license"),
					boolField("eap", "The linter is an EAP build"),
					field("mode", "Native linter or Docker image", fus.EnumExpr("native", "docker")),
				),
				runEvent(baselineResolved, "Baseline the analysis compares its results with is resolved by the CLI: "+
					"a local file, the one of Qodana Cloud for a third-party linter, or none without Qodana Cloud. An IDE "+
					"linter downloads its cloud baseline itself and reports it in qodana.usage.cloud.baseline.resolved, "+
					"joined by the analysis id",
					field("source", "Kind of the baseline, cloud only for a third-party linter",
						fus.EnumExpr("local", "cloud", "none")),
					field("baseline_status", "The baseline is found, or why the analysis runs without it", fus.EnumExpr(
						"found", "missing", "empty", "disabled", "unsupported", "not_connected",
						"timeout", "request_failed", "invalid_response", "io_error",
					)),
					field("http_status", "HTTP status of a failed Qodana Cloud request", integerRule),
					field("request_ms", "Duration of the Qodana Cloud request", integerRule),
				),
				runEvent(analysisStarted, "Analysis started, including git manipulations"),
				runEvent(analysisFinished, "Analysis finished",
					field("exit_code", "Exit code of the analysis", exitCodeRules...),
					field("reason", "Result of the analysis", fus.EnumExpr(reasons...)),
					field("duration_ms", "Duration of the analysis", integerRule),
					field("scenario", "How the analysis runs: on the current state, on a diff of commits, on every "+
						"commit, or on the current state because the start commit of a diff is missing", fus.EnumExpr(
						"default", "scope", "reversed-scope", "full-history", ScenarioStartCommitMissing,
					)),
				),
				runEvent(problemsFound, "Problems of the report and their baseline states, by order of magnitude, "+
					"and whether the analysis applied its baseline",
					field("total", "Problems of the project", fus.EnumExpr(buckets...)),
					field("new", "Problems which are not in the baseline", fus.EnumExpr(buckets...)),
					field("unchanged", "Problems which are in the baseline", fus.EnumExpr(buckets...)),
					field("absent", "Problems of the baseline which are fixed", fus.EnumExpr(buckets...)),
					field("baseline", "The analysis applied the baseline it was given, ignored it, had none, or "+
						"there are no problems to tell. When an IDE linter downloads its cloud baseline itself, the "+
						"value comes from the baseline states of the problems: applied when every problem has one, "+
						"none otherwise", fus.EnumExpr("none", "applied", "ignored", "unknown")),
				),
				runEvent(reportPublished, "Report is published to Qodana Cloud",
					field("publish_status", "The report is uploaded, failed to upload, or not published without a token",
						fus.EnumExpr("uploaded", "failed", "skipped")),
				),
				runEvent(reportShown, "Report is shown by qodana show",
					field("target", "The report of Qodana Cloud, the local one served, or the results directory",
						fus.EnumExpr("cloud", "local", "dir")),
				),
				runEvent(contributorsCounted, "Active contributors are counted by qodana contributors",
					field("contributors", "Active contributors, by order of magnitude", fus.EnumExpr(buckets...)),
					field("repositories", "Repositories counted, by order of magnitude", fus.EnumExpr(buckets...)),
					field("days", "Period the contributors are active in", integerRule),
				),
				runEvent(finished, "Command finished",
					field("exit_code", "Exit code of the command", exitCodeRules...),
					field("reason", "Result of the command", fus.EnumExpr(reasons...)),
					boolField("success", "The command succeeded"),
					field("stage", "Last funnel step the command reached", fus.EnumExpr(
						started, linterSelected, linterDownloaded, linterReady, licenseObtained, licenseRejected,
						baselineResolved, analysisStarted, analysisFinished, problemsFound, reportPublished, reportShown,
						contributorsCounted,
					)),
					field("duration_ms", "Duration of the command", integerRule),
				),
			},
		},
		{
			ID: groupEnv, Version: groupVersion, Description: "Environment Qodana CLI runs in",
			Events: []fus.EventDefinition{
				{ID: "system", Description: "Hardware and OS", Fields: []fus.FieldDefinition{
					field("os", "Operating system", fus.EnumExpr("darwin", "linux", "windows")),
					field("os_version", "Version of the operating system", versionRule, fus.EnumExpr(other)),
					field("distro", "Linux distribution", fus.EnumExpr(append(distros, other)...)),
					field("arch", "CPU architecture", fus.EnumExpr("amd64", "arm64")),
					field("cpu_count", "CPUs, rounded down to a power of two", fus.EnumExpr(cpuCounts...)),
					field("ram_gb", "Memory, of the container when it is limited, rounded down to a power of two",
						fus.EnumExpr(memorySizes...)),
					field("ram_available_gb", "Available memory, rounded down to a power of two", fus.EnumExpr(memorySizes...)),
					field("disk_free_gb", "Free space of the disk of the cache, by order of magnitude", fus.EnumExpr(buckets...)),
				}},
				{ID: "software", Description: "Shell and installed tools", Fields: []fus.FieldDefinition{
					field("shell", "Shell the CLI is started from", fus.EnumExpr(append(shells, other)...)),
					boolField("git", "git is installed"),
					boolField("docker", "Docker CLI is installed"),
					boolField("java", "Java is installed"),
					boolField("tar", "tar is installed"),
					boolField("unzip", "unzip is installed"),
					boolField("toolbox", "JetBrains Toolbox is installed"),
					boolField("ide", "A JetBrains IDE has been run"),
				}},
				{ID: dockerEngine, Description: "Container engine the linter runs in", Fields: []fus.FieldDefinition{
					field("cpu_count", "CPUs of the engine, rounded down to a power of two", fus.EnumExpr(cpuCounts...)),
					field("ram_gb", "Memory of the engine, rounded down to a power of two", fus.EnumExpr(memorySizes...)),
					field("version", "Version of the engine", versionRule, fus.EnumExpr(other)),
				}},
			},
		},
	},
}

// tokenField tells where the Qodana Cloud token of the license comes from.
var tokenField = field("token", "Where the Qodana Cloud token comes from",
	fus.EnumExpr("env", "org_token", "keychain", "prompt"))

// runEvent adds the fields that Finish attaches to every qd.cli.run event.
func runEvent(id string, description string, fields ...fus.FieldDefinition) fus.EventDefinition {
	common := []fus.FieldDefinition{
		field("command", "CLI command", fus.EnumExpr("init", "scan", "send", "show", "contributors")),
		// the ids of Qodana Cloud, hashed already: they join the events of the CLI with its ones
		field("project_id", "Qodana Cloud project id hash, as Qodana Cloud sends it", fus.RegexpExpr(projectIdPattern)),
		field("org_id", "Qodana Cloud organization id hash, as Qodana Cloud sends it", fus.RegexpExpr(orgIdPattern)),
		// linters from 2027.1 send it as system_qdcld_analysis_id: it joins the events of the CLI with the ones of its
		// linters. Older linters don't send it, their events join by the device, the project and the time.
		field("analysis_id", "Analysis id of the scan, which the linter gets too", fus.RegexpExpr(analysisIdPattern)),
	}
	return fus.EventDefinition{ID: id, Description: description, Fields: append(common, fields...)}
}

func field(path string, description string, rules ...string) fus.FieldDefinition {
	return fus.FieldDefinition{Path: path, Description: description, Rules: rules}
}

func boolField(path string, description string) fus.FieldDefinition {
	return field(path, description, fus.EnumRefExpr(enumBoolean))
}

// eventsScheme is the scheme to register with the FUS metadata: the device id is a repository one besides a hash.
func eventsScheme() (*fus.EventsScheme, error) {
	scheme, err := definition.BuildEventsScheme(fus.RecorderConfig{RecorderID: recorderID, RecorderVersion: recorderVersion}, "")
	if err != nil {
		return nil, err
	}
	device := &scheme.RecorderScheme[0].IDs[0]
	device.Values = append(device.Values, "{"+fus.RegexpExpr(repositoryDevice)+"}")
	return scheme, nil
}
