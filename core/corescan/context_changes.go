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
	"github.com/JetBrains/qodana-cli/v2025/core/startup"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2025/platform/strutil"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
)

// All changes of Context must be defined clearly by usecase and business logic
// not just "builder" type functions, but concrete functions related to some business logic
// Why? We want to restrict changes at all, for details see doc on Context
//
// package-internal functions can be "builder" type, like withEnv

func (c Context) WithVcsEnvForFullHistoryAnalysisIteration(remoteUrl string, branch string, revision string) Context {
	return c.
		withEnv(qdenv.QodanaRemoteUrl, remoteUrl, true).
		withEnv(qdenv.QodanaBranch, branch, true).
		withEnv(qdenv.QodanaRevision, revision, true)
}

func (c Context) WithEnvExtractedFromOsEnv(key string, value string) Context {
	return c.withEnv(key, value, false)
}

func (c Context) BackoffToDefaultAnalysisBecauseOfMissingCommit() Context {
	c.commit = ""
	c.diffStart = ""
	c.diffEnd = ""
	c.fullHistory = false
	c.forceLocalChangesScript = false
	c.script = ""
	return c
}

func (c Context) FirstStageOfScopedScript(scopeFile string) Context {
	c.script = strutil.QuoteForWindows("scoped:" + scopeFile)

	startDir := filepath.Join(c.ResultsDir(), "start")
	c = c.prepareContext(
		true,
		"-Dqodana.skip.result=true", // don't print results
		"-Dqodana.skip.coverage.computation=true", // don't compute coverage on first pass
	)
	c.baseline = ""
	c.resultsDir = startDir
	startup.MakeDirAll(c.LogDir()) // need to prepare new result and log dir

	return c
}

func (c Context) SecondStageOfScopedScript(scopeFile string, startSarif string) Context {
	c.script = strutil.QuoteForWindows("scoped:" + scopeFile)

	endDir := filepath.Join(c.ResultsDir(), "end")

	c = c.prepareContext(
		false,
		"-Dqodana.skip.preamble=true", // don't print the QD logo again
		"-Didea.headless.enable.statistics=false",                   // disable statistics for second run
		fmt.Sprintf("-Dqodana.scoped.baseline.path=%s", startSarif), // disable statistics for second run
		"-Dqodana.skip.coverage.issues.reporting=true",              // don't report coverage issues on the second pass, but allow numbers to be computed
	)
	c.resultsDir = endDir
	startup.MakeDirAll(c.LogDir()) // need to prepare new result and log dir
	return c
}

func (c Context) FirstStageOfReverseScopedScript(scopeFile string) Context {
	c.script = strutil.QuoteForWindows("reverse-scoped:NEW," + scopeFile)

	startDir := filepath.Join(c.ResultsDir(), "start")
	properties := []string{"-Dqodana.skip.result.strategy=ANY"} // finish only in case of none issues found

	if !c.BaselineIncludeAbsent() {
		reducedScope := filepath.Join(startDir, "reduced-scope.json")
		properties = append(properties, "-Dqodana.reduced.scope.path="+reducedScope)
		c.reducedScopePath = reducedScope
	}
	c = c.prepareContext(true, properties...)

	c.resultsDir = startDir
	startup.MakeDirAll(c.LogDir()) // need to prepare new result and log dir

	return c
}

func (c Context) SecondStageOfReverseScopedScript(scopeFile string, startSarif string, coveragePath string) Context {
	c.script = strutil.QuoteForWindows("reverse-scoped:OLD," + scopeFile)

	endDir := filepath.Join(c.ResultsDir(), "end")

	resultStrategy := "FIXABLE" // continue if any fixable issues found
	if !c.applyFixes && !c.cleanup {
		resultStrategy = "NEVER" // finish right afterwards
	}
	properties := []string{
		"-Dqodana.skip.preamble=true",             // don't print the QD logo again
		"-Didea.headless.enable.statistics=false", // disable statistics for second run
		fmt.Sprintf("-Dqodana.skip.result.strategy=%s", resultStrategy),
		fmt.Sprintf("-Dqodana.scoped.baseline.path=%s", startSarif),
	}

	c = c.prepareContext(true, properties...)
	c.resultsDir = endDir
	startup.MakeDirAll(c.LogDir()) // need to prepare new result and log dir
	c.copyCoverageFromNewStage(coveragePath)
	return c
}

func (c Context) ThirdStageOfReverseScopedScript(scopeFile string, startSarif string, coveragePath string) Context {
	c.script = strutil.QuoteForWindows("reverse-scoped:FIXES," + scopeFile)

	endDir := filepath.Join(c.ResultsDir(), "fixes")

	properties := []string{
		"-Dqodana.skip.preamble=true",             // don't print the QD logo again
		"-Didea.headless.enable.statistics=false", // disable statistics for second run
		"-Dqodana.skip.result.strategy=NEVER",     // finish right afterwards
		fmt.Sprintf("-Dqodana.scoped.baseline.path=%s", startSarif),
	}

	c = c.prepareContext(false, properties...)
	c.resultsDir = endDir
	startup.MakeDirAll(c.LogDir()) // need to prepare new result and log dir
	c.copyCoverageFromNewStage(coveragePath)
	return c
}

func (c Context) copyCoverageFromNewStage(coverageDataPath string) {
	if info, err := os.Stat(coverageDataPath); err == nil && info.IsDir() {
		startup.MakeDirAll(c.ResultsDir())
		targetCoveragePath := filepath.Join(c.ResultsDir(), "coverage")
		if err := utils.CopyDir(coverageDataPath, targetCoveragePath); err != nil {
			log.Fatalf("Failed to copy coverage data from %s to %s: %v", coverageDataPath, targetCoveragePath, err)
		}
	}
}

func (c Context) ForcedLocalChanges() Context {
	c.script = "local-changes"
	return c
}

// WithEffectiveConfigurationDirOnRevision
// in diff-start, diff-end scenario, for analysis of revision, we reset only
// effectiveConfigurationDir (which is used by IJ), fields used by CLI besides bootstrap stay the same
func (c Context) WithEffectiveConfigurationDirOnRevision(effectiveConfigurationDir string) Context {
	c.effectiveConfigurationDir = effectiveConfigurationDir
	return c
}

func (c Context) withAddedProperties(propertiesToAdd ...string) Context {
	props := c.Property()
	props = append(props, propertiesToAdd...)
	c._property = props
	return c
}

func (c Context) withEnv(key string, value string, override bool) Context {
	currentEnvs := c.Env()
	envs := make([]string, 0)

	for _, e := range currentEnvs {
		isEnvAlreadySet := strings.HasPrefix(e, key) && value != ""
		if isEnvAlreadySet && !override {
			return c
		}

		if !isEnvAlreadySet {
			envs = append(envs, e)
		}
	}
	if value != "" {
		envs = append(envs, fmt.Sprintf("%s=%s", key, value))
	}

	c._env = envs
	return c
}

func (c Context) prepareContext(skipFixes bool, propertiesToAdd ...string) Context {
	c = c.withAddedProperties(propertiesToAdd...)
	c.showReport = false
	c.saveReport = false
	if skipFixes {
		c.applyFixes = false
		c.cleanup = false
		c.fixesStrategy = "none" // this option is deprecated, but the only way to overwrite the possible yaml value
	}
	return c
}
