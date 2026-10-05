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
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/stretchr/testify/assert"
)

func TestWithCloudBaseline(t *testing.T) {
	context := ContextBuilder{ResultsDir: t.TempDir()}.Build().WithCloudBaseline("/data/cache/baseline.sarif.json")

	assert.Equal(t, "/data/cache/baseline.sarif.json", context.Baseline())
	assert.Contains(t, context.Env(), qdenv.QodanaBaselineSource+"="+qdenv.BaselineSourceCloud)
}

// TestFirstStageOfScopedScriptWithoutBaseline verifies that the stage which deliberately runs
// without a baseline doesn't let the linter get one from Qodana Cloud instead.
func TestFirstStageOfScopedScriptWithoutBaseline(t *testing.T) {
	resultsDir := t.TempDir()
	context := ContextBuilder{ResultsDir: resultsDir, LogDir: filepath.Join(resultsDir, "log")}.
		Build().
		WithCloudBaseline("/data/cache/baseline.sarif.json").
		FirstStageOfScopedScript(filepath.Join(resultsDir, "scope.json"))

	assert.Empty(t, context.Baseline(), "the first stage runs without a baseline")
	assert.Contains(t, context.Env(), qdenv.QodanaBaselineSource+"="+qdenv.BaselineSourceNone)
	assert.NotContains(t, context.Env(), qdenv.QodanaBaselineSource+"="+qdenv.BaselineSourceCloud)
}

// TestSecondStageOfScopedScriptKeepsBaseline verifies that the stage which reports the problems
// keeps the baseline the CLI has resolved.
func TestSecondStageOfScopedScriptKeepsBaseline(t *testing.T) {
	resultsDir := t.TempDir()
	context := ContextBuilder{ResultsDir: resultsDir, LogDir: filepath.Join(resultsDir, "log")}.
		Build().
		WithCloudBaseline("/data/cache/baseline.sarif.json").
		SecondStageOfScopedScript(filepath.Join(resultsDir, "scope.json"), filepath.Join(resultsDir, "start.json"))

	assert.Equal(t, "/data/cache/baseline.sarif.json", context.Baseline())
	assert.Contains(t, context.Env(), qdenv.QodanaBaselineSource+"="+qdenv.BaselineSourceCloud)
}
