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
	"github.com/JetBrains/qodana-cli/v2024/core"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/startup"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
)

// newShowCommand returns a new instance of the show command.
func newShowCommand() *cobra.Command {
	cliOptions := &showOptions{}
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a Qodana report",
		Long: `Show (serve) the latest Qodana report. Or open the results directory if the flag is set.

Due to JavaScript security restrictions, the generated report cannot
be viewed via the file:// protocol (by double-clicking the index.html file).
https://www.jetbrains.com/help/qodana/html-report.html
This command serves the Qodana report locally and opens a browser to it.`,
		Run: func(cmd *cobra.Command, args []string) {
			startupArgs := startup.ComputeArgs(
				cliOptions.Linter,
				"",
				"",
				cliOptions.ResultsDir,
				cliOptions.ReportDir,
				os.Getenv(platform.QodanaToken),
				os.Getenv(platform.QodanaLicenseOnlyToken),
				false,
				cliOptions.ProjectDir,
				cliOptions.ConfigName,
			)
			if cliOptions.OpenDir {
				err := core.OpenDir(startupArgs.ResultsDir)
				if err != nil {
					log.Fatal(err)
				}
			} else {
				platform.ShowReport(
					startupArgs.ResultsDir,
					startupArgs.ReportDir,
					cliOptions.Port,
				)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&cliOptions.Linter, "linter", "l", "", "Override linter to use")
	flags.StringVarP(&cliOptions.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVarP(
		&cliOptions.ResultsDir,
		"results-dir",
		"o",
		"",
		"Override directory to save Qodana inspection results to (default <userCacheDir>/JetBrains/<linter>/results)",
	)
	flags.StringVarP(
		&cliOptions.ReportDir,
		"report-dir",
		"r",
		"",
		"Override directory to save Qodana HTML report to (default <userCacheDir>/JetBrains/<linter>/results/report)",
	)
	flags.IntVarP(&cliOptions.Port, "port", "p", 8080, "Specify port to serve report at")
	flags.BoolVarP(&cliOptions.OpenDir, "dir-only", "d", false, "Open report directory only, don't serve it")
	flags.StringVar(
		&cliOptions.ConfigName,
		"config",
		"",
		"Set a custom configuration file instead of 'qodana.yaml'. Relative paths in the configuration will be based on the project directory.",
	)
	return cmd
}

type showOptions struct {
	Linter     string
	ProjectDir string
	ResultsDir string
	ReportDir  string
	Port       int
	OpenDir    bool
	ConfigName string
}
