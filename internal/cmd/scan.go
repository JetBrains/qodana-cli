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

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JetBrains/qodana-cli/internal/cloud"
	"github.com/JetBrains/qodana-cli/internal/core/corescan"
	"github.com/JetBrains/qodana-cli/internal/core/startup"
	"github.com/JetBrains/qodana-cli/internal/platform"
	"github.com/JetBrains/qodana-cli/internal/platform/cmd"
	"github.com/JetBrains/qodana-cli/internal/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/internal/platform/effectiveconfig"
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	log "github.com/sirupsen/logrus"

	"github.com/JetBrains/qodana-cli/internal/core"
	"github.com/spf13/cobra"
)

// newScanCommand returns a new instance of the scan command.
func newScanCommand() *cobra.Command {
	cliOptions := &platformcmd.CliOptions{}
	c := &cobra.Command{
		Use:   "scan",
		Short: "Scan project with Qodana",
		Long: `Scan a project with Qodana. It runs one of Qodana's Docker images (https://www.jetbrains.com/help/qodana/docker-images.html) and reports the results.

Note that most options can be configured via qodana.yaml (https://www.jetbrains.com/help/qodana/qodana-yaml.html) file.
But you can always override qodana.yaml options with the following command-line options.
`,
		Run: func(cmd *cobra.Command, args []string) {
			qdenv.InitializeQodanaGlobalEnv(cliOptions)

			ctx := cmd.Context()

			commonCtx := commoncontext.Compute(
				cliOptions.Linter,
				cliOptions.Ide,
				cliOptions.Image,
				cliOptions.WithinDocker,
				cliOptions.CacheDir,
				cliOptions.ResultsDir,
				cliOptions.ReportDir,
				qdenv.GetQodanaGlobalEnv(qdenv.QodanaToken),
				cliOptions.ClearCache,
				cliOptions.ProjectDir,
				cliOptions.RepositoryRoot,
				cliOptions.ConfigName,
			)
			oldReportUrl := cloud.GetReportUrl(commonCtx.ResultsDir)
			checkProjectDir(commonCtx.ProjectDir)

			preparedHost := startup.PrepareHost(commonCtx)

			effectiveConfigFiles := effectiveconfig.Files{}
			qodanaYamlConfig := corescan.QodanaYamlConfig{}
			if !commonCtx.Analyzer.IsContainer() {
				var err error
				localQodanaYamlFullPath := qdyaml.GetLocalNotEffectiveQodanaYamlFullPath(
					commonCtx.ProjectDir,
					cliOptions.ConfigName,
				)

				effectiveConfigDir, cleanup, err := utils.CreateTempDir("qd-effective-config")
				if err != nil {
					log.Fatalf("Failed to create effective config directory: %v", err)
				}
				defer cleanup()

				effectiveConfigFiles, err = effectiveconfig.CreateEffectiveConfigFiles(
					localQodanaYamlFullPath,
					cliOptions.GlobalConfigurationsDir,
					cliOptions.GlobalConfigurationId,
					preparedHost.Prod.JbrJava(),
					effectiveConfigDir,
					commonCtx.LogDir(),
				)
				if err != nil {
					log.Fatalf("Failed to load Qodana configuration %s", err)
				}
				if effectiveConfigFiles.EffectiveQodanaYamlPath != "" {
					yaml := qdyaml.LoadQodanaYamlByFullPath(effectiveConfigFiles.EffectiveQodanaYamlPath)
					qodanaYamlConfig = corescan.YamlConfig(yaml)
				}
			}
			scanContext := corescan.CreateContext(
				*cliOptions,
				commonCtx,
				preparedHost,
				qodanaYamlConfig,
				effectiveConfigFiles.ConfigDir,
			)

			exitCode := core.RunAnalysis(ctx, scanContext)
			if qdenv.IsContainer() {
				err := platform.ChangeResultsPermissionsRecursively(scanContext.ResultsDir())
				if err != nil {
					msg.ErrorMessage("Unable to change permissions in %s: %s", scanContext.ResultsDir(), err)
				}
			}
			checkExitCode(exitCode, scanContext)
			newReportUrl := cloud.GetReportUrl(scanContext.ResultsDir())
			platform.ProcessSarif(
				filepath.Join(scanContext.ResultsDir(), commoncontext.QodanaSarifName),
				scanContext.AnalysisId(),
				newReportUrl,
				scanContext.PrintProblems(),
				scanContext.GenerateCodeClimateReport(),
				scanContext.SendBitBucketInsights(),
			)

			showReport := scanContext.ShowReport()
			if msg.IsInteractive() {
				showReport = msg.AskUserConfirm("Do you want to open the latest report")
			}

			if newReportUrl != oldReportUrl && newReportUrl != "" && !qdenv.IsContainer() {
				msg.SuccessMessage("Report is successfully uploaded to %s", newReportUrl)
			}

			if showReport {
				commoncontext.ShowReport(scanContext.ResultsDir(), scanContext.ReportDir(), scanContext.Port())
			} else if !qdenv.IsContainer() && msg.IsInteractive() {
				msg.WarningMessage(
					"To view the Qodana report later, run %s in the current directory or add %s flag to %s",
					msg.PrimaryBold("qodana show"),
					msg.PrimaryBold("--show-report"),
					msg.PrimaryBold("qodana scan"),
				)
			}

			if exitCode == utils.QodanaFailThresholdExitCode {
				msg.EmptyMessage()
				msg.ErrorMessage("The number of problems exceeds the fail threshold")
				os.Exit(exitCode)
			}
		},
	}

	err := platformcmd.ComputeFlags(c, cliOptions)
	if err != nil {
		return nil
	}

	return c
}

func checkProjectDir(projectDir string) {
	if msg.IsInteractive() && core.IsHomeDirectory(projectDir) {
		msg.WarningMessage(
			fmt.Sprintf("Project directory (%s) is the $HOME directory", projectDir),
		)
		if !msg.AskUserConfirm(msg.DefaultPromptText) {
			os.Exit(0)
		}
	}
	if !utils.CheckDirFiles(projectDir) {
		msg.ErrorMessage("No files to check with Qodana found in %s", projectDir)
		os.Exit(1)
	}
}

func checkExitCode(exitCode int, c corescan.Context) {
	if exitCode == utils.QodanaEapLicenseExpiredExitCode && msg.IsInteractive() {
		msg.EmptyMessage()
		msg.ErrorMessage(
			"Your license expired: update your license or token. If you are using EAP, make sure you are using the latest CLI version and update to the latest linter by running %s ",
			msg.PrimaryBold("qodana init"),
		)
		os.Exit(exitCode)
	} else if exitCode == utils.QodanaTimeoutExitCodePlaceholder {
		msg.ErrorMessage("Qodana analysis reached timeout %s", c.GetAnalysisTimeout())
		os.Exit(c.AnalysisTimeoutExitCode())
	} else if exitCode == utils.QodanaEmptyChangesetExitCodePlaceholder {
		msg.ErrorMessage("Nothing to analyse. Exiting with %s", utils.QodanaSuccessExitCode)
		os.Exit(utils.QodanaSuccessExitCode)
	} else if exitCode != utils.QodanaSuccessExitCode && exitCode != utils.QodanaFailThresholdExitCode {
		msg.ErrorMessage("Qodana exited with code %d", exitCode)
		msg.WarningMessage("Check ./logs/ in the results directory for more information")
		if exitCode == utils.QodanaOutOfMemoryExitCode {
			msg.ErrorMessage("Qodana was terminated after running out of memory.")
		} else if msg.AskUserConfirm(fmt.Sprintf("Do you want to open %s", c.ResultsDir())) {
			err := core.OpenDir(c.ResultsDir())
			if err != nil {
				log.Fatalf("Error while opening directory: %s", err)
			}
		}
		os.Exit(exitCode)
	}
}
