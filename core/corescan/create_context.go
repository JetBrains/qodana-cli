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
	"github.com/JetBrains/qodana-cli/v2024/core/startup"
	"github.com/JetBrains/qodana-cli/v2024/platform/cmd"
	"github.com/JetBrains/qodana-cli/v2024/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"path/filepath"
	"strings"
)

func CreateContext(
	cliOptions platformcmd.CliOptions,
	commonCtx commoncontext.Context,
	preparedHost startup.PreparedHost,
	qodanaYamlConfig QodanaYamlConfig,
	effectiveConfigurationDir string,
) Context {
	coverageDir := cliOptions.CoverageDir
	if coverageDir == "" {
		if qdenv.IsContainer() {
			coverageDir = "/data/coverage"
		} else {
			coverageDir = filepath.Join(commonCtx.ProjectDir, ".qodana", "code-coverage")
		}
	}

	commit := cliOptions.Commit
	if strings.HasPrefix(commit, "CI") {
		commit = strings.TrimPrefix(commit, "CI")
	}

	return ContextBuilder{
		Linter:                    commonCtx.Linter,
		Ide:                       commonCtx.Ide,
		Id:                        commonCtx.Id,
		IdeDir:                    preparedHost.IdeDir,
		EffectiveConfigurationDir: effectiveConfigurationDir,
		Prod:                      preparedHost.Prod,
		QodanaToken:               preparedHost.QodanaToken,
		QodanaLicenseOnlyToken:    commonCtx.QodanaLicenseOnlyToken,
		ProjectDir:                commonCtx.ProjectDir,
		ResultsDir:                commonCtx.ResultsDir,
		ConfigDir:                 commonCtx.ConfDirPath(),
		LogDir:                    commonCtx.LogDir(),
		QodanaSystemDir:           commonCtx.QodanaSystemDir,
		CacheDir:                  commonCtx.CacheDir,
		ReportDir:                 commonCtx.ReportDir,
		CoverageDir:               coverageDir,
		SourceDirectory:           cliOptions.SourceDirectory,
		Env:                       cliOptions.Env_,
		DisableSanity:             cliOptions.DisableSanity,
		ProfileName:               cliOptions.ProfileName,
		ProfilePath:               cliOptions.ProfilePath,
		RunPromo:                  cliOptions.RunPromo,
		StubProfile:               cliOptions.StubProfile,
		Baseline:                  cliOptions.Baseline,
		BaselineIncludeAbsent:     cliOptions.BaselineIncludeAbsent,
		SaveReport:                cliOptions.SaveReport,
		ShowReport:                cliOptions.ShowReport,
		Port:                      cliOptions.Port,
		Property:                  cliOptions.Property,
		Script:                    cliOptions.Script,
		FailThreshold:             cliOptions.FailThreshold,
		Commit:                    commit,
		DiffStart:                 cliOptions.DiffStart,
		DiffEnd:                   cliOptions.DiffEnd,
		ForceLocalChangesScript:   cliOptions.ForceLocalChangesScript,
		AnalysisId:                cliOptions.AnalysisId,
		Volumes:                   cliOptions.Volumes,
		User:                      cliOptions.User,
		PrintProblems:             cliOptions.PrintProblems,
		GenerateCodeClimateReport: cliOptions.GenerateCodeClimateReport,
		SendBitBucketInsights:     cliOptions.SendBitBucketInsights,
		SkipPull:                  cliOptions.SkipPull,
		ClearCache:                commonCtx.IsClearCache,
		ConfigName:                cliOptions.ConfigName,
		FullHistory:               cliOptions.FullHistory,
		ApplyFixes:                cliOptions.ApplyFixes,
		Cleanup:                   cliOptions.Cleanup,
		FixesStrategy:             cliOptions.FixesStrategy,
		NoStatistics:              cliOptions.NoStatistics,
		CdnetSolution:             cliOptions.CdnetSolution,
		CdnetProject:              cliOptions.CdnetProject,
		CdnetConfiguration:        cliOptions.CdnetConfiguration,
		CdnetPlatform:             cliOptions.CdnetPlatform,
		CdnetNoBuild:              cliOptions.CdnetNoBuild,
		ClangCompileCommands:      cliOptions.ClangCompileCommands,
		ClangArgs:                 cliOptions.ClangArgs,
		AnalysisTimeoutMs:         cliOptions.AnalysisTimeoutMs,
		AnalysisTimeoutExitCode:   cliOptions.AnalysisTimeoutExitCode,
		JvmDebugPort:              cliOptions.JvmDebugPort,
		GlobalConfigurationsFile:  cliOptions.GlobalConfigurationsFile,
		GlobalConfigurationId:     cliOptions.GlobalConfigurationId,
		CustomLocalQodanaYamlPath: cliOptions.ConfigName,
		QodanaYamlConfig:          qodanaYamlConfig,
	}.Build()
}
