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

package scan

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	"github.com/JetBrains/qodana-cli/v2024/platform/product"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdyaml"
	"math"
	"path/filepath"
	"strings"
	"time"
)

const (
	RunScenarioDefault      = "default"
	RunScenarioFullHistory  = "full-history"
	RunScenarioLocalChanges = "local-changes"
	RunScenarioScoped       = "scope"
)

type RunScenario = string

type Context struct {
	Linter                    string
	Ide                       string
	Id                        string
	IdeDir                    string
	QodanaYaml                qdyaml.QodanaYaml
	Prod                      product.Product
	QodanaToken               string
	QodanaLicenseOnlyToken    string
	ProjectDir                string
	ResultsDir                string
	ConfigDir                 string
	LogDir                    string
	QodanaSystemDir           string
	CacheDir                  string
	ReportDir                 string
	CoverageDir               string
	SourceDirectory           string
	_env                      []string
	DisableSanity             bool
	ProfileName               string
	ProfilePath               string
	RunPromo                  string
	StubProfile               string
	Baseline                  string
	BaselineIncludeAbsent     bool
	SaveReport                bool
	ShowReport                bool
	Port                      int
	_property                 []string
	Script                    string
	FailThreshold             string
	Commit                    string
	DiffStart                 string
	DiffEnd                   string
	ForceLocalChangesScript   bool
	AnalysisId                string
	_volumes                  []string
	User                      string
	PrintProblems             bool
	GenerateCodeClimateReport bool
	SendBitBucketInsights     bool
	SkipPull                  bool
	ClearCache                bool
	ConfigName                string
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
}

func (c Context) Env() []string {
	return arrayCopy(c._env)
}

func (c Context) Property() []string {
	return arrayCopy(c._property)
}

func (c Context) Volumes() []string {
	return arrayCopy(c._volumes)
}

func arrayCopy(arr []string) []string {
	newArr := make([]string, len(arr))
	copy(newArr, arr)
	return newArr
}

func (c Context) StartHash() (string, error) {
	switch {
	case c.Commit == c.DiffStart:
		return c.Commit, nil
	case c.Commit == "":
		return c.DiffStart, nil
	case c.DiffStart == "":
		return c.Commit, nil
	default:
		return "", fmt.Errorf("conflicting CLI arguments: --commit=%s --diff-start=%s", c.Commit, c.DiffStart)
	}
}

func (c Context) DetermineRunScenario(hasStartHash bool) RunScenario {
	if c.ForceLocalChangesScript || c.Script == "local-changes" {
		msg.WarningMessage("Using local-changes script is deprecated, please switch to other mechanisms of incremental analysis. Further information - https://www.jetbrains.com/help/qodana/analyze-pr.html")
	}
	switch {
	case c.FullHistory:
		return RunScenarioFullHistory
	case !hasStartHash:
		return RunScenarioDefault
	case c.ForceLocalChangesScript:
		return RunScenarioLocalChanges
	default:
		return RunScenarioScoped
	}
}

func (c Context) IsNative() bool {
	return c.Ide != ""
}

func (c Context) VmOptionsPath() string {
	return filepath.Join(c.ConfigDir, "ide.vmoptions")
}
func (c Context) InstallPluginsVmOptionsPath() string {
	return filepath.Join(c.ConfigDir, "install_plugins.vmoptions")
}

func (c Context) FixesSupported() bool {
	productCode := product.GuessProductCode(c.Ide, c.Linter)
	return productCode != product.QDNET && productCode != product.QDNETC && productCode != product.QDCL
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
	if c.AnalysisTimeoutMs <= 0 {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(c.AnalysisTimeoutMs) * time.Millisecond
}
