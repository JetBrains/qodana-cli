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
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/cli"
	"github.com/JetBrains/qodana-cli/v2024/platform/startup"
	"path/filepath"
)

func CreateContext(
	cliOptions cli.QodanaScanCliOptions,
	startupArgs startup.Args,
	preparedHost startup.PreparedHost,
) Context {
	coverageDir := cliOptions.CoverageDir
	if coverageDir == "" {
		if platform.IsContainer() {
			coverageDir = "/data/coverage"
		} else {
			coverageDir = filepath.Join(startupArgs.ProjectDir, ".qodana", "code-coverage")
		}
	}

	return Context{
		Linter:                    startupArgs.Linter,
		Ide:                       startupArgs.Ide,
		Id:                        startupArgs.Id,
		IdeDir:                    preparedHost.IdeDir,
		QodanaYaml:                platform.QodanaYaml{},
		Prod:                      preparedHost.Prod,
		QodanaToken:               preparedHost.QodanaToken,
		QodanaLicenseOnlyToken:    startupArgs.QodanaLicenseOnlyToken,
		ProjectDir:                startupArgs.ProjectDir,
		ResultsDir:                startupArgs.ResultsDir,
		ConfigDir:                 startupArgs.ConfDirPath(),
		LogDir:                    startupArgs.LogDir(),
		QodanaSystemDir:           startupArgs.QodanaSystemDir,
		CacheDir:                  startupArgs.CacheDir,
		ReportDir:                 startupArgs.ReportDir,
		CoverageDir:               coverageDir,
		SourceDirectory:           cliOptions.SourceDirectory,
		_env:                      cliOptions.Env_,
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
		_property:                 cliOptions.Property,
		Script:                    cliOptions.Script,
		FailThreshold:             cliOptions.FailThreshold,
		Commit:                    cliOptions.Commit,
		DiffStart:                 cliOptions.DiffStart,
		DiffEnd:                   cliOptions.DiffEnd,
		ForceLocalChangesScript:   cliOptions.ForceLocalChangesScript,
		AnalysisId:                cliOptions.AnalysisId,
		_volumes:                  cliOptions.Volumes,
		User:                      cliOptions.User,
		PrintProblems:             cliOptions.PrintProblems,
		GenerateCodeClimateReport: cliOptions.GenerateCodeClimateReport,
		SendBitBucketInsights:     cliOptions.SendBitBucketInsights,
		SkipPull:                  cliOptions.SkipPull,
		ClearCache:                startupArgs.IsClearCache,
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
	}
}
