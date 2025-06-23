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

package core

import (
	"context"
	"github.com/JetBrains/qodana-cli/v2025/core/corescan"
	"github.com/JetBrains/qodana-cli/v2025/platform"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type MockAnalysisRunner struct {
	MockFunc func(hash string, c corescan.Context) (bool, int)
}

func NewMockAnalysisRunner(mockFunc func(hash string, c corescan.Context) (bool, int)) AnalysisRunner {
	return &MockAnalysisRunner{MockFunc: mockFunc}
}

func (r *MockAnalysisRunner) RunFunc(hash string, _ context.Context, c corescan.Context) (bool, int) {
	return r.MockFunc(hash, c)
}

func TestScopedScript(t *testing.T) {
	testCases := []struct {
		name               string
		runFunc            func(hash string, c corescan.Context) (bool, int)
		expectedCalls      int
		expectedHashes     []string
		expectedParamsFunc func(dir string) [][]string
	}{
		{
			name: "successful analysis",
			runFunc: func(hash string, c corescan.Context) (bool, int) {
				return false, 0
			},
			expectedCalls:  2,
			expectedHashes: []string{"startHash", "endHash"},
			expectedParamsFunc: func(dir string) [][]string {
				return [][]string{
					{"-Dqodana.skip.result=true", "-Dqodana.skip.coverage.computation=true"},
					{
						"-Dqodana.skip.preamble=true",
						"-Didea.headless.enable.statistics=false",
						"-Dqodana.scoped.baseline.path=" + filepath.Join(dir, "start", "qodana.sarif.json"),
						"-Dqodana.skip.coverage.issues.reporting=true",
					},
				}
			},
		},
		{
			name: "fail fast",
			runFunc: func(hash string, c corescan.Context) (bool, int) {
				return true, 0
			},
			expectedCalls:  1,
			expectedHashes: []string{"startHash"},
			expectedParamsFunc: func(dir string) [][]string {
				return [][]string{
					{"-Dqodana.skip.result=true", "-Dqodana.skip.coverage.computation=true"},
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(
			tc.name, func(t *testing.T) {
				tempDir, err := os.MkdirTemp("", "qodana-test-*")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}
				defer func(path string) {
					_ = os.RemoveAll(path)
				}(tempDir)

				projectDir := filepath.Join(tempDir, "project")
				resultsDir := filepath.Join(tempDir, "results")
				logDir := filepath.Join(resultsDir, "log")

				c := corescan.ContextBuilder{
					ProjectDir: projectDir,
					ResultsDir: resultsDir,
					LogDir:     logDir,
					Prod: product.Product{
						Code: product.QDJVM,
					},
				}.Build()

				calls := 0
				var hashes []string
				var params [][]string

				runner := NewMockAnalysisRunner(
					func(hash string, c corescan.Context) (bool, int) {
						calls++
						hashes = append(hashes, hash)
						params = append(params, c.Property())
						_ = os.MkdirAll(c.ResultsDir(), 0755)
						return tc.runFunc(hash, c)
					},
				)
				sequenceRunner := &ScopeSequenceRunner{}
				exitCode := sequenceRunner.RunSequence("startHash", "endHash", "scope", context.Background(), c, runner)

				expectedParams := tc.expectedParamsFunc(resultsDir)
				assert.Equal(t, tc.expectedCalls, calls, "Expected %d calls", tc.expectedCalls)
				assert.Equal(t, tc.expectedHashes, hashes, "Expected hashes %v", tc.expectedHashes)
				assert.Equal(t, expectedParams, params, "Expected params %v", expectedParams)
				assert.Equal(t, 0, exitCode, "Expected exit code 0")
			},
		)
	}
}

func TestReverseScopedScript(t *testing.T) {
	testCases := []struct {
		name               string
		runFunc            func(hash string, c corescan.Context) (bool, int)
		fixes              bool
		cleanup            bool
		includeAbsent      bool
		expectedCalls      int
		expectedHashes     []string
		expectedScripts    []string
		expectedParamsFunc func(dir string) [][]string
		createShortSarif   func(path string, count int)
	}{
		{
			name: "successful analysis no fixes",
			runFunc: func(hash string, c corescan.Context) (bool, int) {
				return false, 0
			},
			fixes:           false,
			cleanup:         false,
			includeAbsent:   false,
			expectedCalls:   2,
			expectedHashes:  []string{"endHash", "startHash"},
			expectedScripts: []string{"reverse-scoped:NEW,scope", "reverse-scoped:OLD,reduced-scope.json"},
			expectedParamsFunc: func(dir string) [][]string {
				return [][]string{
					{
						"-Dqodana.skip.result.strategy=ANY",
						"-Dqodana.reduced.scope.path=" + filepath.Join(dir, "reduced-scope.json"),
					},
					{
						"-Dqodana.skip.preamble=true",
						"-Didea.headless.enable.statistics=false",
						"-Dqodana.skip.result.strategy=NEVER",
						"-Dqodana.scoped.baseline.path=" + filepath.Join(dir, "qodana.sarif.json"),
					},
				}
			},
			createShortSarif: func(path string, count int) {
				if count < 2 {
					createShortSarif(path, true)
				} else {
					createShortSarif(path, false)
				}
			},
		},
		{
			name: "successful analysis include absent",
			runFunc: func(hash string, c corescan.Context) (bool, int) {
				return false, 0
			},
			fixes:           false,
			cleanup:         false,
			includeAbsent:   true,
			expectedCalls:   2,
			expectedHashes:  []string{"endHash", "startHash"},
			expectedScripts: []string{"reverse-scoped:NEW,scope", "reverse-scoped:OLD,scope"},
			expectedParamsFunc: func(dir string) [][]string {
				return [][]string{
					{
						"-Dqodana.skip.result.strategy=ANY",
					},
					{
						"-Dqodana.skip.preamble=true",
						"-Didea.headless.enable.statistics=false",
						"-Dqodana.skip.result.strategy=NEVER",
						"-Dqodana.scoped.baseline.path=" + filepath.Join(dir, "qodana.sarif.json"),
					},
				}
			},
			createShortSarif: func(path string, count int) {
				if count < 2 {
					createShortSarif(path, true)
				} else {
					createShortSarif(path, false)
				}
			},
		},
		{
			name: "successful analysis no results",
			runFunc: func(hash string, c corescan.Context) (bool, int) {
				return false, 0
			},
			fixes:           false,
			cleanup:         false,
			includeAbsent:   false,
			expectedCalls:   1,
			expectedHashes:  []string{"endHash"},
			expectedScripts: []string{"reverse-scoped:NEW,scope"},
			expectedParamsFunc: func(dir string) [][]string {
				return [][]string{
					{
						"-Dqodana.skip.result.strategy=ANY",
						"-Dqodana.reduced.scope.path=" + filepath.Join(dir, "reduced-scope.json"),
					},
				}
			},
			createShortSarif: func(path string, count int) {
				createShortSarif(path, false)
			},
		},
		{
			name: "successful analysis with fixes",
			runFunc: func(hash string, c corescan.Context) (bool, int) {
				return false, 0
			},
			fixes:          true,
			cleanup:        false,
			includeAbsent:  false,
			expectedCalls:  3,
			expectedHashes: []string{"endHash", "startHash", "endHash"},
			expectedScripts: []string{
				"reverse-scoped:NEW,scope",
				"reverse-scoped:OLD,reduced-scope.json",
				"reverse-scoped:FIXES,reduced-scope.json",
			},
			expectedParamsFunc: func(dir string) [][]string {
				return [][]string{
					{
						"-Dqodana.skip.result.strategy=ANY",
						"-Dqodana.reduced.scope.path=" + filepath.Join(dir, "reduced-scope.json"),
					},
					{
						"-Dqodana.skip.preamble=true",
						"-Didea.headless.enable.statistics=false",
						"-Dqodana.skip.result.strategy=FIXABLE",
						"-Dqodana.scoped.baseline.path=" + filepath.Join(dir, "qodana.sarif.json"),
					},
					{
						"-Dqodana.skip.preamble=true",
						"-Didea.headless.enable.statistics=false",
						"-Dqodana.skip.result.strategy=NEVER",
						"-Dqodana.scoped.baseline.path=" + filepath.Join(dir, "qodana.sarif.json"),
					},
				}
			},
			createShortSarif: func(path string, count int) {
				if count < 3 {
					createShortSarif(path, true)
				} else {
					createShortSarif(path, false)
				}
			},
		},
		{
			name: "successful analysis with cleanup",
			runFunc: func(hash string, c corescan.Context) (bool, int) {
				return false, 0
			},
			fixes:          false,
			cleanup:        true,
			includeAbsent:  false,
			expectedCalls:  3,
			expectedHashes: []string{"endHash", "startHash", "endHash"},
			expectedScripts: []string{
				"reverse-scoped:NEW,scope",
				"reverse-scoped:OLD,reduced-scope.json",
				"reverse-scoped:FIXES,reduced-scope.json",
			},
			expectedParamsFunc: func(dir string) [][]string {
				return [][]string{
					{
						"-Dqodana.skip.result.strategy=ANY",
						"-Dqodana.reduced.scope.path=" + filepath.Join(dir, "reduced-scope.json"),
					},
					{
						"-Dqodana.skip.preamble=true",
						"-Didea.headless.enable.statistics=false",
						"-Dqodana.skip.result.strategy=FIXABLE",
						"-Dqodana.scoped.baseline.path=" + filepath.Join(dir, "qodana.sarif.json"),
					},
					{
						"-Dqodana.skip.preamble=true",
						"-Didea.headless.enable.statistics=false",
						"-Dqodana.skip.result.strategy=NEVER",
						"-Dqodana.scoped.baseline.path=" + filepath.Join(dir, "qodana.sarif.json"),
					},
				}
			},
			createShortSarif: func(path string, count int) {
				if count < 3 {
					createShortSarif(path, true)
				} else {
					createShortSarif(path, false)
				}
			},
		},
		{
			name: "empty short sarif missing invocation",
			runFunc: func(hash string, c corescan.Context) (bool, int) {
				return false, 0
			},
			fixes:           false,
			cleanup:         false,
			includeAbsent:   true,
			expectedCalls:   2,
			expectedHashes:  []string{"endHash", "startHash"},
			expectedScripts: []string{"reverse-scoped:NEW,scope", "reverse-scoped:OLD,scope"},
			expectedParamsFunc: func(dir string) [][]string {
				return [][]string{
					{
						"-Dqodana.skip.result.strategy=ANY",
					},
					{
						"-Dqodana.skip.preamble=true",
						"-Didea.headless.enable.statistics=false",
						"-Dqodana.skip.result.strategy=NEVER",
						"-Dqodana.scoped.baseline.path=" + filepath.Join(dir, "qodana.sarif.json"),
					},
				}
			},
			createShortSarif: func(path string, count int) {
				createEmptyShortSarif(path)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(
			tc.name, func(t *testing.T) {
				tempDir, err := os.MkdirTemp("", "qodana-test-*")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}
				defer func(path string) {
					_ = os.RemoveAll(path)
				}(tempDir)

				projectDir := filepath.Join(tempDir, "project")
				resultsDir := filepath.Join(tempDir, "results")
				logDir := filepath.Join(resultsDir, "log")

				c := corescan.ContextBuilder{
					ProjectDir: projectDir,
					ResultsDir: resultsDir,
					LogDir:     logDir,
					Prod: product.Product{
						Code: product.QDJVM,
					},
					ApplyFixes:            tc.fixes,
					Cleanup:               tc.cleanup,
					BaselineIncludeAbsent: tc.includeAbsent,
				}.Build()

				calls := 0
				var hashes []string
				var params [][]string
				var scripts []string
				firstDir := ""

				runner := NewMockAnalysisRunner(
					func(hash string, c corescan.Context) (bool, int) {
						calls++
						scripts = append(scripts, normalizeScriptForTesting(c.Script()))
						hashes = append(hashes, hash)
						params = append(params, c.Property())
						if calls == 1 {
							firstDir = c.ResultsDir()
						}
						_ = os.MkdirAll(c.ResultsDir(), 0755)
						tc.createShortSarif(c.ResultsDir(), calls)
						return tc.runFunc(hash, c)
					},
				)
				sequenceRunner := &ReverseScopeSequenceRunner{}
				exitCode := sequenceRunner.RunSequence("startHash", "endHash", "scope", context.Background(), c, runner)

				expectedParams := tc.expectedParamsFunc(firstDir)
				assert.Equal(t, tc.expectedCalls, calls, "Expected %d calls", tc.expectedCalls)
				assert.Equal(t, tc.expectedHashes, hashes, "Expected hashes %v", tc.expectedHashes)
				assert.Equal(t, tc.expectedScripts, scripts, "Expected scripts %v", tc.expectedScripts)
				assert.Equal(t, expectedParams, params, "Expected params %v", expectedParams)
				assert.Equal(t, 0, exitCode, "Expected exit code 0")
			},
		)
	}
}

func normalizeScriptForTesting(script string) string {
	if script == "" {
		return script
	}

	if strings.HasPrefix(script, "reverse-scoped:") {
		parts := strings.SplitN(script, ",", 2)
		if len(parts) == 2 {
			prefix := parts[0]
			filePath := parts[1]
			fileName := filepath.Base(filePath)
			return prefix + "," + fileName
		}
	}

	if strings.HasPrefix(script, "scoped:") {
		filePath := strings.TrimPrefix(script, "scoped:")
		fileName := filepath.Base(filePath)
		return "scoped:" + fileName
	}

	return script
}

func createShortSarif(path string, skipped bool) {
	skippedValue := "false"
	if skipped {
		skippedValue = "true"
	}

	shortSarifContent := `{
				"runs": [
					{
						"invocations": [
							{
								"properties": {
									"qodana.result.skipped": ` + skippedValue + `
								}
							}
						]
					}
				]
			}`
	_ = os.WriteFile(platform.GetShortSarifPath(path), []byte(shortSarifContent), 0644)
}

func createEmptyShortSarif(path string) {
	shortSarifContent := `{
				"runs": [
					{
						"invocations": [{}]
					}
				]
			}`
	_ = os.WriteFile(platform.GetShortSarifPath(path), []byte(shortSarifContent), 0644)
}
