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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JetBrains/qodana-cli/v2025/core/corescan"
	"github.com/JetBrains/qodana-cli/v2025/core/startup"
	"github.com/JetBrains/qodana-cli/v2025/platform"
	"github.com/JetBrains/qodana-cli/v2025/platform/effectiveconfig"
	"github.com/JetBrains/qodana-cli/v2025/platform/git"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	log "github.com/sirupsen/logrus"
)

var defaultRunner = &defaultAnalysisRunner{}

// AnalysisRunner defines the interface for logic on running analysis on commits
type AnalysisRunner interface {
	RunFunc(hash string, ctx context.Context, c corescan.Context) (bool, int)
}

// DefaultAnalysisRunner is the production implementation of RunFunc
type defaultAnalysisRunner struct{}

// SequenceRunner defines the interface for different sequence analysis strategies
type SequenceRunner interface {
	RunSequence(scopeFile string, runner AnalysisRunner) int
	GetParams() (corescan.Context, string, string)
	ComputeEndHash() string
}

// ScopedAnalyzer contains the common logic and coordinates the analysis
type ScopedAnalyzer struct {
	runner         AnalysisRunner
	sequenceRunner SequenceRunner
}

type SequenceRunnerBase struct {
	ctx                context.Context
	c                  corescan.Context
	startHash, endHash string
}

// ScopeSequenceRunner implements the original scope analysis (old commit comes first, new commit comes last)
type ScopeSequenceRunner struct {
	SequenceRunnerBase
}

// ReverseScopeSequenceRunner implements the reverse scope analysis
type ReverseScopeSequenceRunner struct {
	SequenceRunnerBase
}

// NewScopedAnalyzer creates a new ScopedAnalyzer with the original sequence runner
func NewScopedAnalyzer(
	ctx context.Context,
	c corescan.Context,
	startHash, endHash string,
	runner AnalysisRunner,
) *ScopedAnalyzer {
	return &ScopedAnalyzer{
		runner: runner,
		sequenceRunner: &ScopeSequenceRunner{
			SequenceRunnerBase: SequenceRunnerBase{
				ctx:       ctx,
				c:         c,
				startHash: startHash,
				endHash:   endHash,
			},
		},
	}
}

// NewReverseScopedAnalyzer creates a new ScopedAnalyzer with the reversed sequence runner
func NewReverseScopedAnalyzer(
	ctx context.Context,
	c corescan.Context,
	startHash, endHash string,
	runner AnalysisRunner,
) *ScopedAnalyzer {
	var err error
	if endHash == "" {
		endHash, err = git.CurrentRevision(c.RepositoryRoot(), c.LogDir())
		if err != nil {
			log.Fatal(err)
		}
	}
	return &ScopedAnalyzer{
		runner: runner,
		sequenceRunner: &ReverseScopeSequenceRunner{
			SequenceRunnerBase: SequenceRunnerBase{
				ctx:       ctx,
				c:         c,
				startHash: startHash,
				endHash:   endHash,
			},
		},
	}
}

func (sa *ScopedAnalyzer) RunAnalysis() int {
	c, startHash, endHash := sa.sequenceRunner.GetParams()
	var err error
	if startHash == "" || endHash == "" {
		log.Fatal("No commits given. Consider passing --commit or --diff-start and --diff-end (optional) with the range of commits to analyze.")
	}

	changedFiles, err := git.ComputeChangedFiles(c.RepositoryRoot(), startHash, endHash, c.LogDir())
	if err != nil {
		log.Fatal(err)
	}
	if len(changedFiles.Files) == 0 {
		log.Warnf("Nothing to compare between %s and %s", startHash, endHash)
		return utils.QodanaEmptyChangesetExitCodePlaceholder
	}

	scopeFile, err := writeChangesFile(c, changedFiles)
	if err != nil {
		log.Fatal("Failed to prepare diff run ", err)
	}
	defer func() {
		_ = os.Remove(scopeFile)
	}()

	return sa.sequenceRunner.RunSequence(scopeFile, sa.runner)
}

func (r *defaultAnalysisRunner) RunFunc(hash string, ctx context.Context, c corescan.Context) (bool, int) {
	e := git.CheckoutAndUpdateSubmodule(c.RepositoryRoot(), hash, true, c.LogDir())
	if e != nil {
		log.Fatalf("Cannot checkout commit %s: %v", hash, e)
	}

	log.Infof("Analysing %s", hash)

	// for CLI, we use only bootstrap from this effective yaml
	// all other fields are used from the one (effective aswell) obtained at the start
	localQodanaYamlFullPath := qdyaml.GetLocalNotEffectiveQodanaYamlFullPath(
		c.ProjectDir(),
		c.CustomLocalQodanaYamlPath(),
	)
	effectiveConfigDir, cleanup, err := utils.CreateTempDir("qd-effective-config-")
	if err != nil {
		log.Fatalf("Failed to create Qodana effective config directory: %v", err)
	}
	defer cleanup()

	effectiveConfigFiles, err := effectiveconfig.CreateEffectiveConfigFiles(
		localQodanaYamlFullPath,
		c.GlobalConfigurationsDir(),
		c.GlobalConfigurationId(),
		c.Prod().JbrJava(),
		effectiveConfigDir,
		c.LogDir(),
	)
	if err != nil {
		log.Fatalf("Failed to load Qodana configuration during analysis of commit %s: %v", hash, err)
	}

	// if local qodana yaml doesn't exist on revision, for bootstrap fallback to the one constructed at the start
	var bootstrap string
	if c.LocalQodanaYamlExists() {
		yaml := qdyaml.LoadQodanaYamlByFullPath(effectiveConfigFiles.EffectiveQodanaYamlPath)
		bootstrap = yaml.Bootstrap
	} else {
		bootstrap = c.QodanaYamlConfig().Bootstrap
	}
	// TODO: mention that bootstrap should be relative to the project path
	utils.Bootstrap(bootstrap, c.ProjectDir())

	contextForAnalysis := c.WithEffectiveConfigurationDirOnRevision(effectiveConfigFiles.ConfigDir)
	exitCode := runQodana(ctx, contextForAnalysis)
	if !(exitCode == 0 || exitCode == 255) {
		log.Errorf("Qodana analysis on %s exited with code %d. Aborting", hash, exitCode)
		return true, exitCode
	}
	return false, exitCode
}

func (r *ScopeSequenceRunner) RunSequence(
	scopeFile string,
	runner AnalysisRunner,
) int {
	ctx, c, startHash, endHash := computeSequenceParams(&r.SequenceRunnerBase)
	startRunContext := c.FirstStageOfScopedScript(scopeFile)
	stop, code := runner.RunFunc(startHash, ctx, startRunContext)
	if stop {
		return code
	}

	startSarif := platform.GetSarifPath(startRunContext.ResultsDir())

	endRunContext := c.SecondStageOfScopedScript(scopeFile, startSarif)
	stop, code = runner.RunFunc(endHash, ctx, endRunContext)
	if stop {
		return code
	}

	copyAndSaveReport(endRunContext, c)
	return code
}

func (r *ReverseScopeSequenceRunner) RunSequence(
	scopeFile string,
	runner AnalysisRunner,
) int {
	var code int
	var stop bool

	ctx, c, startHash, endHash := computeSequenceParams(&r.SequenceRunnerBase)
	newCodeContext := c.FirstStageOfReverseScopedScript(scopeFile)
	if stop, code = runner.RunFunc(endHash, ctx, newCodeContext); stop {
		return code
	}

	currentContext := newCodeContext
	if shouldProceedToNextStage(currentContext) {
		copyAndSaveReport(currentContext, c)
		return code
	}

	scopeFile, coverageArtifactsPath, newCodeSarif := prepareArtifactPaths(newCodeContext, scopeFile)

	startRunContext := c.SecondStageOfReverseScopedScript(scopeFile, newCodeSarif)
	copyCoverageFromNewStage(coverageArtifactsPath, startRunContext.ResultsDir())
	if stop, code = runner.RunFunc(startHash, ctx, startRunContext); stop {
		return code
	}

	currentContext = startRunContext
	if shouldProceedToNextStage(currentContext) {
		copyAndSaveReport(currentContext, c)
		return code
	}

	if shouldApplyFixes := c.ApplyFixes() || c.Cleanup(); shouldApplyFixes {
		fixesContext := c.ThirdStageOfReverseScopedScript(scopeFile, newCodeSarif)
		copyCoverageFromNewStage(coverageArtifactsPath, fixesContext.ResultsDir())
		if stop, code = runner.RunFunc(endHash, ctx, fixesContext); stop {
			return code
		}
		currentContext = fixesContext
	}

	copyAndSaveReport(currentContext, c)
	return code
}

func computeSequenceParams(r *SequenceRunnerBase) (context.Context, corescan.Context, string, string) {
	return r.ctx, r.c, r.startHash, r.ComputeEndHash()
}

func (r *SequenceRunnerBase) GetParams() (corescan.Context, string, string) {
	return r.c, r.startHash, r.ComputeEndHash()
}

func (r *SequenceRunnerBase) ComputeEndHash() string {
	endHash := r.endHash
	var err error
	if endHash == "" {
		endHash, err = git.CurrentRevision(r.c.RepositoryRoot(), r.c.LogDir())
		if err != nil {
			log.Fatal(err)
		}
		r.endHash = endHash
	}
	return endHash
}

func prepareArtifactPaths(
	newCodeContext corescan.Context,
	originalScopeFile string,
) (scopeFile, coverageArtifactsPath, newCodeSarif string) {
	scopeFile = originalScopeFile
	if reducedPath := newCodeContext.ReducedScopePath(); reducedPath != "" {
		scopeFile = reducedPath
	}

	coverageArtifactsPath = platform.GetCoverageArtifactsPath(newCodeContext.ResultsDir())
	newCodeSarif = platform.GetSarifPath(newCodeContext.ResultsDir())

	return scopeFile, coverageArtifactsPath, newCodeSarif
}

func shouldProceedToNextStage(ctx corescan.Context) bool {
	value := getInvocationProperties(ctx.ResultsDir()).AdditionalProperties["qodana.result.skipped"]
	if strValue, ok := value.(string); ok {
		return strValue == "false"
	}
	if boolValue, ok := value.(bool); ok {
		return !boolValue
	}
	return false
}

func copyAndSaveReport(lastContext corescan.Context, c corescan.Context) {
	err := utils.CopyDir(lastContext.ResultsDir(), c.ResultsDir())
	if err != nil {
		log.Fatal(err)
	}

	saveReport(c)
}

// writeChangesFile creates a temp file containing the changes between diffStart and diffEnd
func writeChangesFile(c corescan.Context, changedFiles git.ChangedFiles) (string, error) {
	file, err := os.CreateTemp("", "diff-scope.txt")
	if err != nil {
		return "", err
	}
	defer func() {
		err := file.Close()
		if err != nil {
			log.Warn("Failed to close scope file ", err)
		}
	}()

	jsonChanges, err := json.MarshalIndent(changedFiles, "", "  ")
	if err != nil {
		return "", err
	}
	_, err = file.WriteString(string(jsonChanges))
	if err != nil {
		return "", fmt.Errorf("failed to write scope file: %w", err)
	}

	err = utils.CopyFile(file.Name(), filepath.Join(c.LogDir(), "changes.json"))
	if err != nil {
		return "", err
	}

	return file.Name(), nil
}

func copyCoverageFromNewStage(coverageDataPath string, resultsDir string) {
	if info, err := os.Stat(coverageDataPath); err == nil && info.IsDir() {
		startup.MakeDirAll(resultsDir)
		targetCoveragePath := filepath.Join(resultsDir, "coverage")
		if err := utils.CopyDir(coverageDataPath, targetCoveragePath); err != nil {
			log.Fatalf("Failed to copy coverage data from %s to %s: %v", coverageDataPath, targetCoveragePath, err)
		}
	}
}
