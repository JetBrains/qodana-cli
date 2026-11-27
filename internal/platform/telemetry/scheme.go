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
	"slices"

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

// Events of the qd.cli.run funnel, in the order a successful native scan emits them: a command starts with its own
// event, like scan.started. Each step has an event of its success and one of its failure, with the reason. The license
// steps are unordered across modes: a third-party linter checks the license before it is ready, and a linter
// container checks it during the analysis, so its license events are timestamped after analysis.started.
const (
	// startedSuffix makes the started event of a command, like scan.started
	startedSuffix      = ".started"
	configResolved     = "config.resolved"
	configFailed       = "config.failed"
	linterSelected     = "linter.selected"
	dockerReady        = "docker.ready"
	dockerFailed       = "docker.failed"
	linterReady        = "linter.ready"
	linterFailed       = "linter.failed"
	licenseObtained    = "license.obtained"
	licenseRejected    = "license.rejected"
	baselineFound      = "baseline.found"
	baselineNone       = "baseline.none"
	baselineFailed     = "baseline.failed"
	analysisStarted    = "analysis.started"
	gitPrepared        = "git.prepared"
	gitFailed          = "git.failed"
	bootstrapSucceeded = "bootstrap.succeeded"
	bootstrapFailed    = "bootstrap.failed"
	analysisSucceeded  = "analysis.succeeded"
	analysisFailed     = "analysis.failed"
	problemsFound      = "problems.found"
	reportPublished    = "report.published"
	publishFailed      = "report.publish_failed"
	commandSucceeded   = "command.succeeded"
	commandFailed      = "command.failed"
	// the steps of qodana show and qodana contributors
	reportShown         = "report.shown"
	showFailed          = "report.show_failed"
	contributorsCounted = "contributors.counted"
)

// commands are the ones that send statistics, each has its started event.
var commands = []string{"init", "scan", "send", "show", "contributors"}

// How the linter is set up, the setup field of linter.ready.
const (
	SetupNativeDownloaded   = "native_downloaded"
	SetupNativeCached       = "native_cached"
	SetupNativePreinstalled = "native_preinstalled"
	SetupDockerPulled       = "docker_pulled"
	SetupDockerCached       = "docker_cached"
	SetupDockerLocal        = "docker_local"
	SetupDockerPullSkipped  = "docker_pull_skipped"
	SetupThirdParty         = "third_party"
)

// The scan_mode of the analysis events: the whole project, a diff of commits, or every commit.
const (
	scanFull        = "full"
	scanIncremental = "incremental"
	scanFullHistory = "full_history"
)

// ScenarioStartCommitMissing is the scenario of an incremental run whose start commit is missing, so it analyzes
// the current state.
const ScenarioStartCommitMissing = "start-commit-missing"

// Results of the succeeded events: the threshold of problems is exceeded, or a diff has no changes.
const (
	ReasonSuccess        = "success"
	ReasonFailThreshold  = "fail_threshold"
	ReasonEmptyChangeset = "empty_changeset"
)

var results = []string{ReasonSuccess, ReasonFailThreshold, ReasonEmptyChangeset}

// Reasons of the failed events of the steps.
const (
	// ReasonDockerUnavailable and the reasons below it are the ones of docker.failed
	ReasonDockerUnavailable = "docker_unavailable"
	ReasonDockerNotLinux    = "docker_not_linux"
	ReasonPullFailed        = "docker_pull_failed"
	// ReasonDownloadFailed is told by command.failed after linter.failed
	ReasonDownloadFailed = "native_download_failed"
	// ReasonReportMissing and ReasonServeFailed are the ones of report.show_failed
	ReasonReportMissing = "report_missing"
	ReasonServeFailed   = "serve_failed"
)

var (
	dockerReasons = []string{ReasonDockerUnavailable, ReasonDockerNotLinux, ReasonPullFailed}
	showReasons   = []string{ReasonReportMissing, ReasonServeFailed}
	// noBaselineReasons are why the analysis has no baseline on purpose, failedBaselineReasons why it failed to get one
	noBaselineReasons     = []string{"not_connected", "disabled", "empty"}
	failedBaselineReasons = []string{"missing", "unsupported", "timeout", "request_failed", "invalid_response", "io_error"}
)

// Reasons of command.failed, which has the ones of the failed steps and of the setup too, and of analysis.failed.
const (
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
	// ReasonSetupError and the reasons below it are failures before the analysis
	ReasonSetupError         = "setup_error"
	ReasonUnsupportedProject = "unsupported_project"
	ReasonInvalidConfig      = "invalid_config"
	ReasonGitMissing         = "git_missing"
	ReasonBootstrapFailed    = "bootstrap_failed"
	ReasonPluginsFailed      = "plugins_failed"
)

// analysisReasons are the reasons that can end an analysis, in a native run and in a linter container alike: of the
// exit code, of a third-party linter, of the steps inside the analysis, of a Docker run, and of an exit during it. A
// report or an upload fails after the analysis. Another one is sent as other.
var analysisReasons = []string{
	ReasonLicenseExpired, ReasonTimeout, ReasonOutOfMemory, ReasonLinterError, ReasonInternalError, ReasonBaselineError,
	ReasonLicenseRejected, ReasonInvalidConfig, ReasonGitMissing, ReasonBootstrapFailed, ReasonPluginsFailed,
	ReasonDockerUnavailable, ReasonDockerNotLinux, ReasonPullFailed, ReasonFatal, ReasonInterrupted,
}

var reasons = append(
	[]string{
		ReasonLicenseExpired, ReasonTimeout, ReasonOutOfMemory, ReasonLinterError, ReasonInternalError,
		ReasonLicenseRejected, ReasonNoFiles, ReasonCancelled, ReasonInterrupted, ReasonFatal, ReasonBaselineError,
		ReasonUploadFailed, ReasonReportFailed, ReasonSetupError, ReasonUnsupportedProject, ReasonInvalidConfig,
		ReasonGitMissing, ReasonBootstrapFailed, ReasonPluginsFailed,
	},
	append(append(slices.Clone(dockerReasons), ReasonDownloadFailed), showReasons...)...,
)

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
	// licensePlans are the qodana_license_type of resources.jetbrains.com/storage/ap/fus/metadata/tiger/QD/groups/QD.json,
	// listed in the field so that the scheme shows them: a new plan is sent as other until it is added here
	licensePlans = []string{
		"EAP_COMMUNITY", "EAP_ULTIMATE", "EAP_ULTIMATE_PLUS", "TRIAL_COMMUNITY", "TRIAL_ULTIMATE",
		"TRIAL_ULTIMATE_PLUS", "COMMUNITY", "ULTIMATE", "ULTIMATE_PLUS", "PREMIUM", "PREMIUM_LITE",
	}
	cpuCounts   = []string{"1", "2", "4", "8", "16", "32", "64+"}
	memorySizes = []string{"0", "1", "2", "4", "8", "16", "32", "64", "128+"}
)

// Global rules of the FUS metadata, the only rules a field refers to. The values are local copies for the
// validation before sending, the server validates with its own: a copy may accept less, never more.
const (
	enumBoolean   = "boolean"
	regexpInteger = "integer"
	regexpVersion = "version"
)

var globalRules = &fus.SchemeRules{
	Enums: map[string][]string{
		enumBoolean: {"true", "false"},
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
			Events: runEvents(
				runEvent(configResolved, "Qodana configuration is loaded: the qodana.yaml of the project, a global "+
					"configuration, both, or none. The linter reports the settings in qodana.usage.config",
					field("source", "Where the configuration comes from", fus.EnumExpr(
						"local", "global", "local_and_global", "none",
					)),
				),
				runEvent(configFailed, "Qodana configuration can't be loaded: command.failed tells invalid_config"),
				runEvent(linterSelected, "Linter is selected for the project by qodana init",
					field("linter", "Product code of the linter", fus.EnumExpr(linters...)),
					field("mode", "Native linter or Docker image, absent for a linter name in qodana.yaml: qodana "+
						"scan chooses its mode", fus.EnumExpr("native", "docker")),
					boolField("preconfigured", "qodana.yaml of the project has the linter already"),
				),
				runEvent(linterReady, "Linter is on the machine and chosen for the analysis",
					append(slices.Clone(linterFields),
						field("setup", "How the linter is set up: downloaded, cached or preinstalled native, the Docker "+
							"image pulled, up to date, local after a failed pull, or not pulled with --skip-pull, or a "+
							"third-party linter", fus.EnumExpr(
							SetupNativeDownloaded, SetupNativeCached, SetupNativePreinstalled, SetupDockerPulled,
							SetupDockerCached, SetupDockerLocal, SetupDockerPullSkipped, SetupThirdParty,
						)),
						field("download_ms", "Duration of the download or the pull, 0 without one", integerRule),
					)...,
				),
				runEvent(linterFailed, "Native linter can't be downloaded: command.failed tells native_download_failed. "+
					"Docker problems are of docker.failed",
					field("linter", "Product code of the linter", fus.EnumExpr(linters...)),
				),
				runEvent(dockerReady, "Container engine is reachable and runs Linux: its limits, not the host ones, "+
					"apply to the linter",
					field("cpu_count", "CPUs of the engine, rounded down to a power of two", fus.EnumExpr(cpuCounts...)),
					field("ram_gb", "Memory of the engine, rounded down to a power of two", fus.EnumExpr(memorySizes...)),
					field("version", "Version of the engine", versionRule, fus.EnumExpr(other)),
				),
				runEvent(dockerFailed, "Docker run of the linter can't start: the engine is unreachable, doesn't run "+
					"Linux, or the image can't be pulled",
					field("linter", "Product code of the linter of the image", fus.EnumExpr(linters...)),
					field("reason", "Why the Docker run can't start", fus.EnumExpr(dockerReasons...)),
				),
				runEvent(licenseObtained, "License obtained",
					tokenField,
					field("kind", "Where the license comes from: env is the license key of QODANA_LICENSE, community a "+
						"free linter, eap an EAP build, cloud the license of the Qodana Cloud token",
						fus.EnumExpr("env", "community", "eap", "cloud")),
					field("plan", "License plan of the Qodana Cloud organization",
						fus.EnumExpr(append(slices.Clone(licensePlans), other)...)),
				),
				runEvent(licenseRejected, "License rejected",
					field("reason", "Why the license is rejected", fus.EnumExpr(
						"no_token", "token_invalid", "token_declined", "request_failed", "community_plan", "empty_key", "expired",
					)),
					tokenField,
				),
				runEvent(baselineFound, "Baseline the analysis compares its results with is found by the CLI: a local "+
					"file, or the one of Qodana Cloud for a third-party linter. An IDE linter downloads its cloud baseline "+
					"itself and reports it in qodana.usage.cloud.baseline.resolved, joined by the analysis id",
					field("source", "Kind of the baseline, cloud only for a third-party linter",
						fus.EnumExpr("local", "cloud")),
					requestField,
				),
				runEvent(baselineNone, "Analysis runs without a baseline on purpose: no Qodana Cloud and no baseline "+
					"file, or the cloud baseline of the project is turned off or empty",
					field("reason", "Why the analysis runs without a baseline: not_connected without Qodana Cloud, "+
						"disabled or empty for the cloud baseline", fus.EnumExpr(noBaselineReasons...)),
					requestField,
				),
				runEvent(baselineFailed, "Baseline can't be used: the file is missing, or the one of Qodana Cloud "+
					"can't be downloaded",
					field("source", "Kind of the baseline", fus.EnumExpr("local", "cloud")),
					field("reason", "Why the baseline can't be used", fus.EnumExpr(failedBaselineReasons...)),
					field("http_status", "HTTP status of a failed Qodana Cloud request", integerRule),
					requestField,
				),
				runEvent(analysisStarted, "Analysis started: an incremental one checks its start commit in "+
					"git.prepared, then checks out the commits it analyzes", scanModeField),
				runEvent(gitPrepared, "Start commit of an incremental or full-history analysis is checked, before "+
					"the commits are checked out",
					field("start_commit", "The start commit is in the repository, or missing: the analysis runs on "+
						"the current state", fus.EnumExpr("found", "missing")),
				),
				runEvent(gitFailed, "git is needed by the analysis and is not installed: command.failed tells git_missing"),
				runEvent(bootstrapSucceeded, "bootstrap command of qodana.yaml ran before the analysis",
					field("duration_ms", "Duration of the bootstrap command", integerRule),
				),
				runEvent(bootstrapFailed, "bootstrap command of qodana.yaml failed",
					field("exit_code", "Exit code of the bootstrap command", exitCodeRules...),
					field("duration_ms", "Duration of the bootstrap command", integerRule),
				),
				runEvent(analysisSucceeded, "Analysis finished with a report",
					append(slices.Clone(analysisFields), resultField)...,
				),
				runEvent(analysisFailed, "Analysis failed, also when the CLI exits during it",
					append(slices.Clone(analysisFields), field("reason", "Why the analysis failed: interrupted comes "+
						"with exit_code 0, the CLI exits with 0 when it is interrupted",
						fus.EnumExpr(append(slices.Clone(analysisReasons), other)...)))...),
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
				runEvent(reportPublished, "Report is uploaded to Qodana Cloud. A run without a Qodana Cloud token, "+
					"which has no project_id, publishes nothing"),
				runEvent(publishFailed, "Report can't be uploaded to Qodana Cloud. A full-history Docker run uploads the "+
					"report of each commit, so it can have both report.published and report.publish_failed"),
				runEvent(reportShown, "Report is shown by qodana show",
					field("target", "The report of Qodana Cloud, the local one served, or the results directory",
						fus.EnumExpr("cloud", "local", "dir")),
				),
				runEvent(showFailed, "Report can't be shown by qodana show",
					field("reason", "Why the report can't be shown", fus.EnumExpr(showReasons...)),
				),
				runEvent(contributorsCounted, "Active contributors are counted by qodana contributors",
					field("contributors", "Active contributors, by order of magnitude", fus.EnumExpr(buckets...)),
					field("project_dirs", "Project directories given with --project-dir, by order of magnitude",
						fus.EnumExpr(buckets...)),
					field("days", "Period the contributors are active in", integerRule),
				),
				runEvent(commandSucceeded, "Command succeeded", append(slices.Clone(finishFields), resultField)...),
				runEvent(commandFailed, "Command failed",
					append(slices.Clone(finishFields), field("reason", "Why the command failed: interrupted comes with "+
						"exit_code 0, the CLI exits with 0 when it is interrupted", fus.EnumExpr(reasons...)))...,
				),
			),
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
			},
		},
	},
}

var (
	commandField = field("command", "CLI command", fus.EnumExpr(commands...))
	requestField = field("request_ms", "Duration of the Qodana Cloud request", integerRule)
	// linterFields are known when the linter is chosen, a native one tells its version once it is downloaded
	linterFields = []fus.FieldDefinition{
		field("linter", "Product code of the linter", fus.EnumExpr(linters...)),
		field("version", "Version of the linter, latest for an image without a version", versionRule,
			fus.EnumExpr("latest", other)),
		boolField("paid", "The linter requires a license"),
		boolField("eap", "The linter is an EAP build"),
	}
	scanModeField = field("scan_mode", "The analysis runs on the whole project, on a diff of commits, or on every commit",
		fus.EnumExpr(scanFull, scanIncremental, scanFullHistory))
	analysisFields = []fus.FieldDefinition{
		field("linter", "Product code of the linter, as linter.ready tells it", fus.EnumExpr(linters...)),
		field("mode", "Native linter, Docker image this CLI starts, third-party linter, or container: the CLI runs "+
			"inside a Qodana image", fus.EnumExpr("native", "docker", "third_party", "container")),
		field("exit_code", "Exit code of the analysis: a success can come with a non-zero one, over the fail threshold "+
			"or when a linter container can't write the report", exitCodeRules...),
		field("duration_ms", "Duration of the analysis. A Docker one includes the pull of the image, see the "+
			"download_ms of linter.ready, and the start of the container", integerRule),
		scanModeField,
		field("scenario", "Diff of an incremental analysis: the start commit first, the end commit first, or the "+
			"current state because the start commit is missing", fus.EnumExpr(
			"scope", "reversed-scope", ScenarioStartCommitMissing,
		)),
	}
	finishFields = []fus.FieldDefinition{
		commandField,
		field("exit_code", "Exit code of the command: a success can come with a non-zero one, over the fail threshold",
			exitCodeRules...),
		field("stage", "Last funnel step the command reached", fus.EnumExpr(append(
			startedEvents(),
			configResolved, configFailed, linterSelected, dockerReady, dockerFailed, linterReady, linterFailed,
			licenseObtained, licenseRejected,
			baselineFound, baselineNone, baselineFailed, analysisStarted, gitPrepared, gitFailed, bootstrapSucceeded,
			bootstrapFailed, analysisSucceeded, analysisFailed, problemsFound, reportPublished, publishFailed,
			reportShown, showFailed, contributorsCounted,
		)...)),
		field("duration_ms", "Duration of the command", integerRule),
	}
)

// resultField is the result of the succeeded events: fail_threshold is a success, the tool worked, though the CLI
// exits with a non-zero code.
var resultField = field("result", "Result: success, fail_threshold, a success though the CLI exits with a non-zero "+
	"code as the problems exceed the threshold, or empty_changeset, a diff without changes", fus.EnumExpr(results...))

// tokenField tells where the Qodana Cloud token of the license comes from.
var tokenField = field("token", "Where the Qodana Cloud token comes from: env is QODANA_TOKEN, org_token one "+
	"exchanged for the organization token, keychain the one qodana login saved, prompt the one the user typed",
	fus.EnumExpr("env", "org_token", "keychain", "prompt"))

// startedEvents are the started events of the commands, like scan.started.
func startedEvents() []string {
	events := make([]string, 0, len(commands))
	for _, command := range commands {
		events = append(events, command+startedSuffix)
	}
	return events
}

// runEvents are the events of qd.cli.run: the started events of the commands, then the steps.
func runEvents(steps ...fus.EventDefinition) []fus.EventDefinition {
	events := make([]fus.EventDefinition, 0, len(commands)+len(steps))
	for _, command := range commands {
		fields := []fus.FieldDefinition{
			boolField("container", "The CLI runs in a Qodana container"),
			boolField("interactive", "The CLI runs in an interactive terminal"),
			field("ci", "CI system the CLI runs on, none outside CI", fus.EnumExpr(append(ciNames, "none", other)...)),
		}
		events = append(events, runEvent(command+startedSuffix, "qodana "+command+" started", fields...))
	}
	return append(events, steps...)
}

// runEvent adds the fields that Finish attaches to every qd.cli.run event. The finish events tell the command too,
// the other events of a run share the session of its started event.
func runEvent(id string, description string, fields ...fus.FieldDefinition) fus.EventDefinition {
	common := []fus.FieldDefinition{
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
