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

package thirdpartyscan

import (
	"github.com/JetBrains/qodana-cli/v2024/platform/cmd"
	"github.com/JetBrains/qodana-cli/v2024/platform/platforminit"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdyaml"
	"path/filepath"
	"strings"
)

func ComputeContext(
	cliOptions platformcmd.CliOptions,
	initArgs platforminit.Args,
	linterInfo LinterInfo,
	mountInfo MountInfo,
	cloudData ThirdPartyStartupCloudData,
	qodanaYaml qdyaml.QodanaYaml,
) Context {
	projectDir := initArgs.ProjectDir

	clangCompileCommands := cliOptions.ClangCompileCommands
	if strings.HasPrefix(clangCompileCommands, "./") || strings.HasPrefix(clangCompileCommands, "../") {
		clangCompileCommands = filepath.Join(projectDir, clangCompileCommands)
		clangCompileCommands = filepath.Clean(clangCompileCommands)
	}

	clangArgs := cliOptions.ClangArgs
	if clangArgs != "" {
		clangArgs = "-- " + clangArgs
	}

	return ContextBuilder{
		LinterInfo:            linterInfo,
		MountInfo:             mountInfo,
		CloudData:             cloudData,
		ProjectDir:            projectDir,
		ResultsDir:            initArgs.ResultsDir,
		LogDir:                initArgs.LogDir(),
		CacheDir:              initArgs.CacheDir,
		ClangCompileCommands:  clangCompileCommands,
		ClangArgs:             clangArgs,
		Property:              cliOptions.Property,
		CdnetSolution:         cliOptions.CdnetSolution,
		CdnetProject:          cliOptions.CdnetProject,
		CdnetConfiguration:    cliOptions.CdnetConfiguration,
		CdnetPlatform:         cliOptions.CdnetPlatform,
		NoStatistics:          cliOptions.NoStatistics,
		CdnetNoBuild:          cliOptions.CdnetNoBuild,
		AnalysisId:            cliOptions.AnalysisId,
		Baseline:              cliOptions.Baseline,
		BaselineIncludeAbsent: cliOptions.BaselineIncludeAbsent,
		FailThreshold:         cliOptions.FailThreshold,
		QodanaYaml:            qodanaYaml,
	}.Build()
}
