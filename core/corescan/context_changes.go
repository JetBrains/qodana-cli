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
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
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
	c.script = utils.QuoteForWindows("scoped:" + scopeFile)

	startDir := filepath.Join(c.ResultsDir(), "start")
	c.showReport = false
	c.showReport = false
	c = c.withAddedProperties(
		"-Dqodana.skip.result=true",               // don't print results
		"-Dqodana.skip.coverage.computation=true", // don't compute coverage on first pass
	)
	c.baseline = ""
	c.resultsDir = startDir
	c.applyFixes = false
	c.cleanup = false
	c.fixesStrategy = "none" // this option is deprecated, but the only way to overwrite the possible yaml value
	return c
}

func (c Context) SecondStageOfScopedScript(scopeFile string, startSarif string) Context {
	c.script = utils.QuoteForWindows("scoped:" + scopeFile)

	endDir := filepath.Join(c.ResultsDir(), "end")
	c = c.withAddedProperties(
		"-Dqodana.skip.preamble=true",                               // don't print the QD logo again
		"-Didea.headless.enable.statistics=false",                   // disable statistics for second run
		fmt.Sprintf("-Dqodana.scoped.baseline.path=%s", startSarif), // disable statistics for second run
		"-Dqodana.skip.coverage.issues.reporting=true",              // don't report coverage issues on the second pass, but allow numbers to be computed
	)
	c.resultsDir = endDir
	c.showReport = false
	c.saveReport = false
	return c
}

func (c Context) ForcedLocalChanges() Context {
	c.script = "local-changes"
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
