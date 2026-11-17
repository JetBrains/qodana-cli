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
	"path"
	"path/filepath"

	"github.com/JetBrains/qodana-cli/internal/cloud"
	"github.com/JetBrains/qodana-cli/internal/core/corescan"
	"github.com/JetBrains/qodana-cli/internal/core/exitcodes"
	"github.com/JetBrains/qodana-cli/internal/core/startup"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	"github.com/JetBrains/qodana-cli/internal/platform"
	platformcmd "github.com/JetBrains/qodana-cli/internal/platform/cmd"
	"github.com/JetBrains/qodana-cli/internal/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/internal/platform/effectiveconfig"
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/qdcontainer"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/internal/platform/telemetry"
	"github.com/JetBrains/qodana-cli/internal/platform/tokenloader"
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
			telemetry.Start("scan", func() string { return platform.GetDeviceIdSalt(cliOptions.ProjectDir)[0] }, cliOptions.NoStatistics)
			tokenloader.InitializeQodanaGlobalEnv(cliOptions)
			// QODANA_ORG_TOKEN is resolved: drop it so it never reaches the scan context and its debug output
			cliOptions.Env_ = qdenv.WithoutEnv(cliOptions.Env_, qdenv.QodanaOrgToken)

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

				effectiveConfigDir, cleanup, err := fs.CreateTempDir("qd-effective-config")
				if err != nil {
					log.Fatalf("Failed to create effective config directory: %v", err)
				}
				defer cleanup()

				effectiveConfigFiles, err = effectiveconfig.CreateEffectiveConfigFiles(
					commonCtx.CacheDir,
					localQodanaYamlFullPath,
					cliOptions.GlobalConfigurationsDir,
					cliOptions.GlobalConfigurationId,
					effectiveConfigDir,
					commonCtx.LogDir(),
				)
				if err != nil {
					telemetry.Fail(telemetry.ReasonInvalidConfig)
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

			baseline := platform.ResolveBaseline(
				scanContext.Baseline(),
				scanContext.ProjectDir(),
				platform.LauncherBaselineSource(),
				scanContext.QodanaUploadToken(),
				scanContext.Analyser().GetLinter().ProductCode,
				scanContext.CacheDir(),
			)
			if baseline.IsFromCloud() {
				scanContext = scanContext.WithCloudBaseline(baselineForLinter(baseline.BaselinePath(), scanContext))
			} else {
				scanContext = scanContext.WithResolvedBaselineSource(baseline.Source())
			}
			// set over a source of the environment, which qdenv.SetEnv would keep
			if err := os.Setenv(qdenv.QodanaBaselineSource, baseline.Source()); err != nil {
				log.Warnf("Failed to set %s: %v", qdenv.QodanaBaselineSource, err)
			}

			finishAnalysis := telemetry.StartAnalysis()
			exitCode := core.RunAnalysis(ctx, scanContext)
			finishAnalysis(statisticsExit(exitCode, scanContext))
			// not deferred: the exit code checks below leave the process with os.Exit
			baseline.Cleanup()
			if baseline.ReportsUsed(scanContext.Analyser().IsContainer()) {
				fmt.Println(baseline.UsedMessage())
			}
			if qdenv.IsContainer() {
				err := platform.ChangeResultsPermissionsRecursively(scanContext.ResultsDir())
				if err != nil {
					msg.ErrorMessage("Unable to change permissions in %s: %s", scanContext.ResultsDir(), err)
				}
			}
			newReportUrl := cloud.GetReportUrl(scanContext.ResultsDir())
			if exitCode == exitcodes.QodanaSuccessExitCode || exitCode == exitcodes.QodanaFailThresholdExitCode {
				// the linter publishes the report itself and leaves its url in the results
				telemetry.ReportPublished(
					scanContext.QodanaUploadToken() != "",
					newReportUrl != "" && newReportUrl != oldReportUrl,
				)
			} else {
				// checkExitCode exits
				telemetry.Finish(statisticsExit(exitCode, scanContext))
			}
			checkExitCode(exitCode, scanContext)
			platform.ProcessSarif(
				filepath.Join(scanContext.ResultsDir(), commoncontext.QodanaSarifName),
				scanContext.AnalysisId(),
				newReportUrl,
				scanContext.PrintProblems(),
				scanContext.GenerateCodeClimateReport(),
				scanContext.SendBitBucketInsights(),
			)
			telemetry.Finish(exitCode, "")

			if newReportUrl != oldReportUrl && newReportUrl != "" && !qdenv.IsContainer() {
				msg.SuccessMessage("Report is successfully uploaded to %s", newReportUrl)
			}

			commoncontext.InteractiveShowReport(
				scanContext.ShowReport(),
				scanContext.CacheDir(),
				scanContext.ResultsDir(),
				scanContext.ReportDir(),
				scanContext.ShowReportPort(),
			)
			if exitCode == exitcodes.QodanaFailThresholdExitCode {
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
			telemetry.Finish(0, telemetry.ReasonCancelled)
			os.Exit(0)
		}
	}
	if !fs.CheckDirFiles(projectDir) {
		msg.ErrorMessage("No files to check with Qodana found in %s", projectDir)
		telemetry.Finish(1, telemetry.ReasonNoFiles)
		os.Exit(1)
	}
}

// statisticsExit returns the exit code checkExitCode exits with for a placeholder, and its reason.
func statisticsExit(exitCode int, c corescan.Context) (int, string) {
	switch exitCode {
	case exitcodes.QodanaTimeoutExitCodePlaceholder:
		return c.AnalysisTimeoutExitCode(), telemetry.ReasonTimeout
	case exitcodes.QodanaEmptyChangesetExitCodePlaceholder:
		return exitcodes.QodanaSuccessExitCode, telemetry.ReasonEmptyChangeset
	}
	return exitCode, ""
}

func checkExitCode(exitCode int, c corescan.Context) {
	if exitCode == exitcodes.QodanaEapLicenseExpiredExitCode && msg.IsInteractive() {
		msg.EmptyMessage()
		msg.ErrorMessage(
			"Your license expired: update your license or token. If you are using EAP, make sure you are using the latest CLI version and update to the latest linter by running %s ",
			msg.PrimaryBold("qodana init"),
		)
		os.Exit(exitCode)
	} else if exitCode == exitcodes.QodanaTimeoutExitCodePlaceholder {
		msg.ErrorMessage("Qodana analysis reached timeout %s", c.GetAnalysisTimeout())
		os.Exit(c.AnalysisTimeoutExitCode())
	} else if exitCode == exitcodes.QodanaEmptyChangesetExitCodePlaceholder {
		msg.ErrorMessage("Nothing to analyse. Exiting with %s", exitcodes.QodanaSuccessExitCode)
		os.Exit(exitcodes.QodanaSuccessExitCode)
	} else if exitCode != exitcodes.QodanaSuccessExitCode && exitCode != exitcodes.QodanaFailThresholdExitCode {
		msg.ErrorMessage("Qodana exited with code %d", exitCode)
		msg.WarningMessage("Check ./logs/ in the results directory for more information")
		if exitCode == exitcodes.QodanaOutOfMemoryExitCode {
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

// baselineForLinter returns the path the linter reads the downloaded baseline from. A linter in a
// container reads it from the cache dir mounted into the container.
func baselineForLinter(baselinePath string, c corescan.Context) string {
	if !c.Analyser().IsContainer() {
		return baselinePath
	}
	relative, err := filepath.Rel(c.CacheDir(), baselinePath)
	if err != nil {
		log.Fatalf("Baseline %s is not in the cache dir %s: %v", baselinePath, c.CacheDir(), err)
	}
	return path.Join(qdcontainer.DataCacheDir, filepath.ToSlash(relative))
}
