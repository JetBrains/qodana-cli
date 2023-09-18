/*
 * Copyright 2021-2023 JetBrains s.r.o.
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
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/JetBrains/qodana-cli/core"
	"github.com/spf13/cobra"
)

// newScanCommand returns a new instance of the scan command.
func newScanCommand() *cobra.Command {
	options := &core.QodanaOptions{}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan project with Qodana",
		Long: `Scan a project with Qodana. It runs one of Qodana's Docker images (https://www.jetbrains.com/help/qodana/docker-images.html) and reports the results.

Note that most options can be configured via qodana.yaml (https://www.jetbrains.com/help/qodana/qodana-yaml.html) file.
But you can always override qodana.yaml options with the following command-line options.
`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			checkProjectDir(options.ProjectDir)
			if options.YamlName == "" {
				options.YamlName = core.FindQodanaYaml(options.ProjectDir)
			}

			if options.Linter == "" && options.Ide == "" {
				qodanaYaml := core.LoadQodanaYaml(options.ProjectDir, options.YamlName)
				if qodanaYaml.Linter == "" && qodanaYaml.Ide == "" {
					core.WarningMessage(
						"No valid `linter:` field found in %s. Have you run %s? Running that for you...",
						core.PrimaryBold(options.YamlName),
						core.PrimaryBold("qodana init"),
					)
					options.Linter = core.GetLinter(options.ProjectDir, options.YamlName)
					core.EmptyMessage()
				} else {
					options.Linter = qodanaYaml.Linter
				}
				if options.Ide == "" {
					options.Ide = qodanaYaml.Ide
				}
			}
			exitCode := core.RunAnalysis(ctx, options)

			checkExitCode(exitCode, options.ResultsDir)
			core.ReadSarif(filepath.Join(options.ResultsDir, core.QodanaSarifName), options.PrintProblems)
			if core.IsInteractive() {
				options.ShowReport = core.AskUserConfirm("Do you want to open the latest report")
			}

			if options.ShowReport {
				core.ShowReport(options.ResultsDir, options.ReportDirPath(), options.Port)
			} else {
				core.WarningMessage(
					"To view the Qodana report later, run %s in the current directory or add %s flag to %s",
					core.PrimaryBold("qodana show"),
					core.PrimaryBold("--show-report"),
					core.PrimaryBold("qodana scan"),
				)
			}

			if exitCode == core.QodanaFailThresholdExitCode {
				core.EmptyMessage()
				core.ErrorMessage("The number of problems exceeds the fail threshold")
				os.Exit(exitCode)
			}
		},
	}

	flags := cmd.Flags()
	flags.SortFlags = false

	if !core.IsContainer() {
		flags.StringVarP(&options.Linter, "linter", "l", "", "Use to run Qodana in a container (default). Choose linter (image) to use. Not compatible with --ide option. Available images are: "+strings.Join(core.AllImages, ", "))
	}
	flags.StringVar(&options.Ide, "ide", os.Getenv(core.QodanaDistEnv), fmt.Sprintf("Use to run Qodana without a container. Path to the installed IDE, or a downloaded one: provide direct URL or a product code. Not compatible with --linter option. Available codes are %s, add -EAP part to obtain EAP versions", strings.Join(core.AllSupportedCodes, ", ")))

	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVarP(&options.ResultsDir, "results-dir", "o", options.ResultsDirPath(), "Override directory to save Qodana inspection results to")
	flags.StringVar(&options.CacheDir, "cache-dir", options.CacheDirPath(), "Override cache directory (default <userCacheDir>/JetBrains/<linter>/cache)")
	flags.StringVarP(&options.ReportDir, "report-dir", "r", options.ReportDirPath(), "Override directory to save Qodana HTML report to")

	flags.BoolVar(&options.PrintProblems, "print-problems", false, "Print all found problems by Qodana in the CLI output")
	flags.BoolVar(&options.ClearCache, "clear-cache", false, "Clear the local Qodana cache before running the analysis")
	flags.BoolVarP(&options.ShowReport, "show-report", "w", false, "Serve HTML report on port")
	flags.IntVar(&options.Port, "port", 8080, "Port to serve the report on")
	flags.StringVar(&options.YamlName, "yaml-name", "", "Override qodana.yaml name to use: 'qodana.yaml' or 'qodana.yml'")

	flags.StringVarP(&options.AnalysisId, "analysis-id", "a", uuid.New().String(), "Unique report identifier (GUID) to be used by Qodana Cloud")
	flags.StringVarP(&options.Baseline, "baseline", "b", "", "Provide the path to an existing SARIF report to be used in the baseline state calculation")
	flags.BoolVar(&options.BaselineIncludeAbsent, "baseline-include-absent", false, "Include in the output report the results from the baseline run that are absent in the current run")
	flags.BoolVar(&options.FullHistory, "full-history", false, "Go through the full commit history and run the analysis on each commit. If combined with `--commit`, analysis will be started from the given commit. Could take a long time.")
	flags.StringVar(&options.Commit, "commit", "", "Base changes commit to reset to, resets git and runs linter with `--script local-changes`: analysis will be run only on changed files since the given commit. If combined with `--full-history`, full history analysis will be started from the given commit.")
	flags.StringVar(&options.FailThreshold, "fail-threshold", "", "Set the number of problems that will serve as a quality gate. If this number is reached, the inspection run is terminated with a non-zero exit code")
	flags.BoolVar(&options.DisableSanity, "disable-sanity", false, "Skip running the inspections configured by the sanity profile")
	flags.StringVarP(&options.SourceDirectory, "source-directory", "d", "", "Directory inside the project-dir directory must be inspected. If not specified, the whole project is inspected")
	flags.StringVarP(&options.ProfileName, "profile-name", "n", "", "Profile name defined in the project")
	flags.StringVarP(&options.ProfilePath, "profile-path", "p", "", "Path to the profile file")
	flags.StringVar(&options.RunPromo, "run-promo", "", "Set to 'true' to have the application run the inspections configured by the promo profile; set to 'false' otherwise (default: 'true' only if Qodana is executed with the default profile)")
	flags.StringVar(&options.Script, "script", "default", "Override the run scenario")
	flags.StringVar(&options.StubProfile, "stub-profile", "", "Absolute path to the fallback profile file. This option is applied in case the profile was not specified using any available options")

	flags.BoolVar(&options.ApplyFixes, "apply-fixes", false, "Apply all available quick-fixes, including cleanup")
	flags.BoolVar(&options.Cleanup, "cleanup", false, "Run project cleanup")
	flags.StringVar(&options.FixesStrategy, "fixes-strategy", "", "Set the strategy for applying quick-fixes. Available values: 'apply', 'cleanup', 'none'")

	flags.StringArrayVar(&options.Property, "property", []string{}, "Set a JVM property to be used while running Qodana using the --property property.name=value1,value2,...,valueN notation")
	flags.BoolVarP(&options.SaveReport, "save-report", "s", true, "Generate HTML report")

	if !core.IsContainer() {
		flags.StringArrayVarP(&options.Env, "env", "e", []string{}, "Only for container runs. Define additional environment variables for the Qodana container (you can use the flag multiple times). CLI is not reading full host environment variables and does not pass it to the Qodana container for security reasons")
		flags.StringArrayVarP(&options.Volumes, "volume", "v", []string{}, "Only for container runs. Define additional volumes for the Qodana container (you can use the flag multiple times)")
		flags.StringVarP(&options.User, "user", "u", core.GetDefaultUser(), "Only for container runs. User to run Qodana container as. Please specify user id – '$UID' or user id and group id $(id -u):$(id -g). Use 'root' to run as the root user (default: the current user)")
		flags.BoolVar(&options.SkipPull, "skip-pull", false, "Only for container runs. Skip pulling the latest Qodana container")
		cmd.MarkFlagsMutuallyExclusive("linter", "ide")
		cmd.MarkFlagsMutuallyExclusive("skip-pull", "ide")
		cmd.MarkFlagsMutuallyExclusive("volume", "ide")
		cmd.MarkFlagsMutuallyExclusive("user", "ide")
		cmd.MarkFlagsMutuallyExclusive("env", "ide")
	}

	cmd.MarkFlagsMutuallyExclusive("commit", "script")
	cmd.MarkFlagsMutuallyExclusive("profile-name", "profile-path")
	cmd.MarkFlagsMutuallyExclusive("apply-fixes", "cleanup")

	err := cmd.Flags().MarkHidden("fixes-strategy")
	if err != nil {
		return nil
	}
	err = cmd.Flags().MarkDeprecated("fixes-strategy", "use --apply-fixes / --cleanup instead")
	if err != nil {
		return nil
	}

	return cmd
}

func checkProjectDir(projectDir string) {
	if core.IsInteractive() && core.IsHomeDirectory(projectDir) {
		core.WarningMessage(
			fmt.Sprintf("Project directory (%s) is the $HOME directory", projectDir),
		)
		if !core.AskUserConfirm(core.DefaultPromptText) {
			os.Exit(0)
		}
	}
	if !core.CheckDirFiles(projectDir) {
		core.ErrorMessage("No files to check with Qodana found in %s", projectDir)
		os.Exit(1)
	}
}

func checkExitCode(exitCode int, resultsDir string) {
	if exitCode == core.QodanaEapLicenseExpiredExitCode && core.IsInteractive() {
		core.EmptyMessage()
		core.ErrorMessage(
			"Your license expired: update your license or token. If you are using EAP, make sure you are using the latest CLI version and update to the latest linter by running %s ",
			core.PrimaryBold("qodana init"),
		)
		os.Exit(exitCode)
	} else if exitCode != core.QodanaSuccessExitCode && exitCode != core.QodanaFailThresholdExitCode {
		core.ErrorMessage("Qodana exited with code %d", exitCode)
		core.WarningMessage("Check ./logs/ in the results directory for more information")
		if exitCode == core.QodanaOutOfMemoryExitCode {
			core.CheckContainerEngineMemory()
		} else if core.AskUserConfirm(fmt.Sprintf("Do you want to open %s", resultsDir)) {
			err := core.OpenDir(resultsDir)
			if err != nil {
				log.Fatalf("Error while opening directory: %s", err)
			}
		}
		os.Exit(exitCode)
	}
}
