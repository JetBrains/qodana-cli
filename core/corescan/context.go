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

package corescan

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdyaml"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	RunScenarioDefault        = "default"
	RunScenarioFullHistory    = "full-history"
	RunScenarioLocalChanges   = "local-changes"
	RunScenarioScoped         = "scope"
	RunScenarioReversedScoped = "reversed-scope"
)

type RunScenario = string

// Context
//
// If one has the instance of Context, then it means that it was initialized, and it is in valid state
// all mutations should be defined in context_changes.go with names clearly demonstrating the usecase and business logic.
// example: scoped script launches two stages of default analysis
//
// # In the future, you can consider making it an interface
//
// Yes, there is a lot of boilerplate, and it's ok, it's much better than having one object written and initialized across your program
// if I have an object it means it must be in valid state, otherwise it's impossible to reason about the code
//
// Q: - Why do we prohibit mutations in Context if we pass it by value already?
// A: - Because we want to prohibit any unexpected changes to Context at all, imagine at some place the projectDir will
//
//	suddenly change, and pass it further? It's not clear why it was changed? Was it actually initialized at this place?
//	but it must be initialized already, so we limit all changes, and keep them in context_changes.go
//
// !!!KEEP IT IMMUTABLE!!!
type Context struct {
	analyser                  product.Analyzer
	id                        string
	ideDir                    string
	effectiveConfigurationDir string
	globalConfigurationsDir   string
	globalConfigurationId     string
	customLocalQodanaYamlPath string
	qodanaYamlConfig          QodanaYamlConfig
	prod                      product.Product
	qodanaUploadToken         string
	projectDir                string
	resultsDir                string
	configDir                 string
	logDir                    string
	qodanaSystemDir           string
	cacheDir                  string
	reportDir                 string
	coverageDir               string
	sourceDirectory           string
	_env                      []string
	disableSanity             bool
	profileName               string
	profilePath               string
	runPromo                  string
	baseline                  string
	baselineIncludeAbsent     bool
	saveReport                bool
	showReport                bool
	port                      int
	_property                 []string
	script                    string
	failThreshold             string
	commit                    string
	diffStart                 string
	diffEnd                   string
	forceLocalChangesScript   bool
	reversePrAnalysis         bool
	reducedScopePath          string
	analysisId                string
	_volumes                  []string
	user                      string
	printProblems             bool
	generateCodeClimateReport bool
	sendBitBucketInsights     bool
	skipPull                  bool
	fullHistory               bool
	applyFixes                bool
	cleanup                   bool
	fixesStrategy             string
	noStatistics              bool
	cdnetSolution             string
	cdnetProject              string
	cdnetConfiguration        string
	cdnetPlatform             string
	cdnetNoBuild              bool
	clangCompileCommands      string
	clangArgs                 string
	analysisTimeoutMs         int
	analysisTimeoutExitCode   int
	jvmDebugPort              int
}

// QodanaYamlConfig fields from qodana.yaml used in CLI for core linters (also `linter` and `ide`)
type QodanaYamlConfig struct {
	Bootstrap  string
	Plugins    []qdyaml.Plugin
	Properties map[string]string
	DotNet     qdyaml.DotNet
}

func YamlConfig(yaml qdyaml.QodanaYaml) QodanaYamlConfig {
	return QodanaYamlConfig{
		Bootstrap:  yaml.Bootstrap,
		Plugins:    yaml.Plugins,
		Properties: yaml.Properties,
		DotNet:     yaml.DotNet,
	}
}

func (c Context) Analyser() product.Analyzer         { return c.analyser }
func (c Context) Id() string                         { return c.id }
func (c Context) IdeDir() string                     { return c.ideDir }
func (c Context) EffectiveConfigurationDir() string  { return c.effectiveConfigurationDir }
func (c Context) GlobalConfigurationsDir() string    { return c.globalConfigurationsDir }
func (c Context) GlobalConfigurationId() string      { return c.globalConfigurationId }
func (c Context) CustomLocalQodanaYamlPath() string  { return c.customLocalQodanaYamlPath }
func (c Context) QodanaYamlConfig() QodanaYamlConfig { return c.qodanaYamlConfig }
func (c Context) Prod() product.Product              { return c.prod }
func (c Context) QodanaUploadToken() string          { return c.qodanaUploadToken }
func (c Context) ProjectDir() string                 { return c.projectDir }
func (c Context) ResultsDir() string                 { return c.resultsDir }
func (c Context) ConfigDir() string                  { return c.configDir }
func (c Context) LogDir() string                     { return c.logDir }
func (c Context) QodanaSystemDir() string            { return c.qodanaSystemDir }
func (c Context) CacheDir() string                   { return c.cacheDir }
func (c Context) ReportDir() string                  { return c.reportDir }
func (c Context) CoverageDir() string                { return c.coverageDir }
func (c Context) SourceDirectory() string            { return c.sourceDirectory }
func (c Context) DisableSanity() bool                { return c.disableSanity }
func (c Context) ProfileName() string                { return c.profileName }
func (c Context) ProfilePath() string                { return c.profilePath }
func (c Context) RunPromo() string                   { return c.runPromo }
func (c Context) Baseline() string                   { return c.baseline }
func (c Context) BaselineIncludeAbsent() bool        { return c.baselineIncludeAbsent }
func (c Context) SaveReport() bool                   { return c.saveReport }
func (c Context) ShowReport() bool                   { return c.showReport }
func (c Context) Port() int                          { return c.port }
func (c Context) Script() string                     { return c.script }
func (c Context) FailThreshold() string              { return c.failThreshold }
func (c Context) Commit() string                     { return c.commit }
func (c Context) DiffStart() string                  { return c.diffStart }
func (c Context) DiffEnd() string                    { return c.diffEnd }
func (c Context) ForceLocalChangesScript() bool      { return c.forceLocalChangesScript }
func (c Context) ReducedScopePath() string           { return c.reducedScopePath }
func (c Context) ReversePrAnalysis() bool            { return c.reversePrAnalysis }
func (c Context) AnalysisId() string                 { return c.analysisId }
func (c Context) User() string                       { return c.user }
func (c Context) PrintProblems() bool                { return c.printProblems }
func (c Context) GenerateCodeClimateReport() bool    { return c.generateCodeClimateReport }
func (c Context) SendBitBucketInsights() bool        { return c.sendBitBucketInsights }
func (c Context) SkipPull() bool                     { return c.skipPull }
func (c Context) FullHistory() bool                  { return c.fullHistory }
func (c Context) ApplyFixes() bool                   { return c.applyFixes }
func (c Context) Cleanup() bool                      { return c.cleanup }
func (c Context) FixesStrategy() string              { return c.fixesStrategy }
func (c Context) NoStatistics() bool                 { return c.noStatistics }
func (c Context) CdnetSolution() string              { return c.cdnetSolution }
func (c Context) CdnetProject() string               { return c.cdnetProject }
func (c Context) CdnetConfiguration() string         { return c.cdnetConfiguration }
func (c Context) CdnetPlatform() string              { return c.cdnetPlatform }
func (c Context) CdnetNoBuild() bool                 { return c.cdnetNoBuild }
func (c Context) ClangCompileCommands() string       { return c.clangCompileCommands }
func (c Context) ClangArgs() string                  { return c.clangArgs }
func (c Context) AnalysisTimeoutMs() int             { return c.analysisTimeoutMs }
func (c Context) AnalysisTimeoutExitCode() int       { return c.analysisTimeoutExitCode }
func (c Context) JvmDebugPort() int                  { return c.jvmDebugPort }
func (c Context) Env() []string                      { return arrayCopy(c._env) }
func (c Context) Property() []string                 { return arrayCopy(c._property) }
func (c Context) Volumes() []string                  { return arrayCopy(c._volumes) }

type ContextBuilder struct {
	Analyser                  product.Analyzer
	Id                        string
	IdeDir                    string
	EffectiveConfigurationDir string
	Prod                      product.Product
	QodanaUploadToken         string
	ProjectDir                string
	ResultsDir                string
	ConfigDir                 string
	LogDir                    string
	QodanaSystemDir           string
	CacheDir                  string
	ReportDir                 string
	CoverageDir               string
	SourceDirectory           string
	Env                       []string
	DisableSanity             bool
	ProfileName               string
	ProfilePath               string
	RunPromo                  string
	Baseline                  string
	BaselineIncludeAbsent     bool
	SaveReport                bool
	ShowReport                bool
	Port                      int
	Property                  []string
	Script                    string
	FailThreshold             string
	Commit                    string
	DiffStart                 string
	DiffEnd                   string
	ForceLocalChangesScript   bool
	ReversePrAnalysis         bool
	AnalysisId                string
	Volumes                   []string
	User                      string
	PrintProblems             bool
	GenerateCodeClimateReport bool
	SendBitBucketInsights     bool
	SkipPull                  bool
	FullHistory               bool
	ApplyFixes                bool
	Cleanup                   bool
	FixesStrategy             string
	NoStatistics              bool
	CdnetSolution             string
	CdnetProject              string
	CdnetConfiguration        string
	CdnetPlatform             string
	CdnetNoBuild              bool
	ClangCompileCommands      string
	ClangArgs                 string
	AnalysisTimeoutMs         int
	AnalysisTimeoutExitCode   int
	JvmDebugPort              int
	GlobalConfigurationsDir   string
	GlobalConfigurationId     string
	CustomLocalQodanaYamlPath string
	QodanaYamlConfig          QodanaYamlConfig
}

func (b ContextBuilder) Build() Context {
	return Context{
		analyser:                  b.Analyser,
		id:                        b.Id,
		ideDir:                    b.IdeDir,
		effectiveConfigurationDir: b.EffectiveConfigurationDir,
		prod:                      b.Prod,
		qodanaUploadToken:         b.QodanaUploadToken,
		projectDir:                b.ProjectDir,
		resultsDir:                b.ResultsDir,
		configDir:                 b.ConfigDir,
		logDir:                    b.LogDir,
		qodanaSystemDir:           b.QodanaSystemDir,
		cacheDir:                  b.CacheDir,
		reportDir:                 b.ReportDir,
		coverageDir:               b.CoverageDir,
		sourceDirectory:           b.SourceDirectory,
		_env:                      b.Env,
		disableSanity:             b.DisableSanity,
		profileName:               b.ProfileName,
		profilePath:               b.ProfilePath,
		runPromo:                  b.RunPromo,
		baseline:                  b.Baseline,
		baselineIncludeAbsent:     b.BaselineIncludeAbsent,
		saveReport:                b.SaveReport,
		showReport:                b.ShowReport,
		port:                      b.Port,
		_property:                 b.Property,
		script:                    b.Script,
		failThreshold:             b.FailThreshold,
		commit:                    b.Commit,
		diffStart:                 b.DiffStart,
		diffEnd:                   b.DiffEnd,
		forceLocalChangesScript:   b.ForceLocalChangesScript,
		reversePrAnalysis:         b.ReversePrAnalysis,
		analysisId:                b.AnalysisId,
		_volumes:                  b.Volumes,
		user:                      b.User,
		printProblems:             b.PrintProblems,
		generateCodeClimateReport: b.GenerateCodeClimateReport,
		sendBitBucketInsights:     b.SendBitBucketInsights,
		skipPull:                  b.SkipPull,
		fullHistory:               b.FullHistory,
		applyFixes:                b.ApplyFixes,
		cleanup:                   b.Cleanup,
		fixesStrategy:             b.FixesStrategy,
		noStatistics:              b.NoStatistics,
		cdnetSolution:             b.CdnetSolution,
		cdnetProject:              b.CdnetProject,
		cdnetConfiguration:        b.CdnetConfiguration,
		cdnetPlatform:             b.CdnetPlatform,
		cdnetNoBuild:              b.CdnetNoBuild,
		clangCompileCommands:      b.ClangCompileCommands,
		clangArgs:                 b.ClangArgs,
		analysisTimeoutMs:         b.AnalysisTimeoutMs,
		analysisTimeoutExitCode:   b.AnalysisTimeoutExitCode,
		jvmDebugPort:              b.JvmDebugPort,
		globalConfigurationsDir:   b.GlobalConfigurationsDir,
		globalConfigurationId:     b.GlobalConfigurationId,
		customLocalQodanaYamlPath: b.CustomLocalQodanaYamlPath,
		qodanaYamlConfig:          b.QodanaYamlConfig,
	}
}

func arrayCopy(arr []string) []string {
	newArr := make([]string, len(arr))
	copy(newArr, arr)
	return newArr
}

func (c Context) StartHash() (string, error) {
	switch {
	case c.Commit() == c.DiffStart():
		return c.Commit(), nil
	case c.Commit() == "":
		return c.DiffStart(), nil
	case c.DiffStart() == "":
		return c.Commit(), nil
	default:
		return "", fmt.Errorf("conflicting CLI arguments: --commit=%s --diff-start=%s", c.Commit(), c.DiffStart())
	}
}

func (c Context) DetermineRunScenario(hasStartHash bool) RunScenario {
	if c.ForceLocalChangesScript() || c.Script() == "local-changes" {
		msg.WarningMessage("Using local-changes script is deprecated, please switch to other mechanisms of incremental analysis. Further information - https://www.jetbrains.com/help/qodana/analyze-pr.html")
	}
	switch {
	case c.FullHistory():
		return RunScenarioFullHistory
	case !hasStartHash:
		return RunScenarioDefault
	case c.ForceLocalChangesScript():
		return RunScenarioLocalChanges
	case c.analyser.IsContainer():
		return RunScenarioDefault
	case c.ReversePrAnalysis():
		return RunScenarioReversedScoped
	default:
		return RunScenarioScoped
	}
}

func (c Context) VmOptionsPath() string {
	return filepath.Join(c.ConfigDir(), "ide.vmoptions")
}
func (c Context) InstallPluginsVmOptionsPath() string {
	return filepath.Join(c.ConfigDir(), "install_plugins.vmoptions")
}

func (c Context) PropertiesAndFlags() (map[string]string, []string) {
	var flagsArr []string
	props := map[string]string{}
	for _, arg := range c.Property() {
		kv := strings.SplitN(arg, "=", 2)
		if len(kv) == 2 {
			props[kv[0]] = kv[1]
		} else {
			flagsArr = append(flagsArr, arg)
		}
	}
	return props, flagsArr
}

func (c Context) GetAnalysisTimeout() time.Duration {
	if c.AnalysisTimeoutMs() <= 0 {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(c.AnalysisTimeoutMs()) * time.Millisecond
}

func (c Context) LocalQodanaYamlExists() bool {
	path := qdyaml.GetLocalNotEffectiveQodanaYamlFullPath(c.ProjectDir(), c.CustomLocalQodanaYamlPath())
	if path == "" {
		return false
	}
	info, _ := os.Stat(path)
	return info != nil
}

func IsScopedScenario(scenario string) bool {
	return scenario == RunScenarioScoped || scenario == RunScenarioReversedScoped
}
