/*
 * Copyright 2021-2022 JetBrains s.r.o.
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
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/JetBrains/qodana-cli/core"
	"github.com/spf13/cobra"
)

// NewScanCommand returns a new instance of the scan command.
func NewScanCommand() *cobra.Command {
	options := &core.QodanaOptions{}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan project with Qodana",
		Long: `Scan a project with Qodana. It runs one of Qodana Docker's images (https://www.jetbrains.com/help/qodana/docker-images.html) and reports the results.

Note that most options can be configured via qodana.yaml (https://www.jetbrains.com/help/qodana/qodana-yaml.html) file.
But you can always override qodana.yaml options with the following command-line options.
`,
		PreRun: func(cmd *cobra.Command, args []string) {
			core.CheckDockerHost()
		},
		Run: func(cmd *cobra.Command, args []string) {
			go core.CheckForUpdates(core.Version)
			if !core.CheckDirFiles(options.ProjectDir) {
				core.ErrorMessage("No files to check with Qodana found in %s", options.ProjectDir)
				os.Exit(1)
			}
			gitReset := false
			if options.Commit != "" && core.IsGitInstalled() {
				err := core.GitReset(options.ProjectDir, options.Commit)
				if err != nil {
					core.WarningMessage("Could not reset git repository, no --commit option will be applied: %s", err)
				} else {
					gitReset = true
				}
			}
			ctx := cmd.Context()
			if options.YamlName == "" {
				options.YamlName = core.FindQodanaYaml(options.ProjectDir)
			}
			if options.Linter == "" {
				qodanaYaml := core.LoadQodanaYaml(options.ProjectDir, options.YamlName)
				if qodanaYaml.Linter == "" {
					core.WarningMessage(
						"No valid qodana.yaml found. Have you run %s? Running that for you...",
						core.PrimaryBold("qodana init"),
					)
					options.Linter = core.GetLinter(options.ProjectDir, options.YamlName)
					core.EmptyMessage()
				} else {
					options.Linter = qodanaYaml.Linter
				}
			}
			core.PrepareHost(options)
			exitCode := core.RunLinter(ctx, options)
			if core.Interrupted {
				os.Exit(1)
			}
			if exitCode != core.QodanaSuccessExitCode && exitCode != core.QodanaFailThresholdExitCode {
				core.ErrorMessage("Qodana exited with code %d", exitCode)
				core.WarningMessage("Please check the logs in %s", options.ResultsDir)
				if core.IsInteractive() {
					ready, err := core.QodanaInteractiveConfirm.Show()
					if err != nil {
						log.Fatalf("Error while waiting for user input: %s", err)
					}
					if ready {
						err = core.OpenDir(options.ResultsDir)
						if err != nil {
							log.Fatalf("Error while opening directory: %s", err)
						}
					}
				}
				os.Exit(exitCode)
			}
			problems := core.ReadSarif(filepath.Join(options.ResultsDir, "qodana.sarif.json"), options.PrintProblems)
			if problems == 0 {
				core.SuccessMessage("It seems all right 👌 No problems found according to the checks applied")
			} else {
				core.ErrorMessage("Qodana found %d problems according to the checks applied", problems)
			}
			if gitReset && !strings.HasPrefix(options.Commit, "CI") {
				_ = core.GitResetBack(options.ProjectDir)
			}
			if options.ShowReport {
				core.ShowReport(filepath.Join(options.ResultsDir, "report"), options.Port)
			} else if core.IsInteractive() {
				core.WarningMessage(
					"To view the Qodana report, run %s or add %s flag to %s",
					core.PrimaryBold("qodana show"),
					core.PrimaryBold("--show-report"),
					core.PrimaryBold("qodana scan"),
				)
			}
			if exitCode == core.QodanaFailThresholdExitCode {
				core.EmptyMessage()
				core.ErrorMessage("The number of problems exceeds the failThreshold")
				os.Exit(exitCode)
			}
		},
	}

	flags := cmd.Flags()

	flags.StringVarP(&options.Linter, "linter", "l", "", "Override linter to use")
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVarP(&options.ResultsDir, "results-dir", "o", "", "Override directory to save Qodana inspection results to (default <userCacheDir>/JetBrains/<linter>/results)")
	flags.StringVar(&options.CacheDir, "cache-dir", "", "Override cache directory (default <userCacheDir>/JetBrains/<linter>/cache)")
	flags.StringArrayVarP(&options.Env, "env", "e", []string{}, "Define additional environment variables for the Qodana container (you can use the flag multiple times). CLI is not reading full host environment variables and does not pass it to the Qodana container for security reasons")
	flags.StringArrayVarP(&options.Volumes, "volume", "v", []string{}, "Define additional volumes for the Qodana container (you can use the flag multiple times)")
	flags.StringVarP(&options.User, "user", "u", "", "User to run Qodana container as. Please specify user id – '$UID' or user id and group id $(id -u):$(id -g). Use 'root' to run as the root user (default: the current user)")
	flags.BoolVar(&options.SkipPull, "skip-pull", false, "Skip pulling the latest Qodana container")
	flags.BoolVar(&options.PrintProblems, "print-problems", false, "Print all found problems by Qodana in the CLI output")
	flags.BoolVar(&options.ClearCache, "clear-cache", false, "Clear the local Qodana cache before running the analysis")
	flags.BoolVarP(&options.ShowReport, "show-report", "w", false, "Serve HTML report on port")
	flags.IntVar(&options.Port, "port", 8080, "Port to serve the report on")
	flags.StringVar(&options.YamlName, "yaml-name", "", "Override qodana.yaml name to use: 'qodana.yaml' or 'qodana.yml'")

	flags.StringVarP(&options.AnalysisId, "analysis-id", "a", "", "Unique report identifier (GUID) to be used by Qodana Cloud")
	flags.StringVarP(&options.Baseline, "baseline", "b", "", "Provide the path to an existing SARIF report to be used in the baseline state calculation")
	flags.BoolVar(&options.BaselineIncludeAbsent, "baseline-include-absent", false, "Include in the output report the results from the baseline run that are absent in the current run")
	flags.BoolVarP(&options.Changes, "changes", "c", false, "Inspect uncommitted changes and report new problems")
	flags.StringVar(&options.Commit, "commit", "", "Base changes commit to reset to, useful with --changes: analysis will be run only on changed files since commit X, 'reset' will be cancelled once the analysis is finished if the commit prefix does not contain CI prefix")
	flags.StringVar(&options.FailThreshold, "fail-threshold", "", "Set the number of problems that will serve as a quality gate. If this number is reached, the inspection run is terminated with a non-zero exit code")
	flags.BoolVar(&options.DisableSanity, "disable-sanity", false, "Skip running the inspections configured by the sanity profile")
	flags.StringVarP(&options.SourceDirectory, "source-directory", "d", "", "Directory inside the project-dir directory must be inspected. If not specified, the whole project is inspected")
	flags.StringVarP(&options.ProfileName, "profile-name", "n", "", "Profile name defined in the project")
	flags.StringVarP(&options.ProfilePath, "profile-path", "p", "", "Path to the profile file")
	flags.StringVar(&options.RunPromo, "run-promo", "", "Set to 'true' to have the application run the inspections configured by the promo profile; set to 'false' otherwise (default: 'true' only if Qodana is executed with the default profile)")
	flags.StringVar(&options.Script, "script", "default", "Override the run scenario")
	flags.StringVar(&options.StubProfile, "stub-profile", "", "Absolute path to the fallback profile file. This option is applied in case the profile was not specified using any available options")

	flags.StringArrayVar(&options.Property, "property", []string{}, "Set a JVM property to be used while running Qodana using the --property property.name=value1,value2,...,valueN notation")
	flags.BoolVarP(&options.SaveReport, "save-report", "s", true, "Generate HTML report")

	flags.SortFlags = false

	return cmd
}
