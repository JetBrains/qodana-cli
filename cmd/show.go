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
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// newShowCommand returns a new instance of the show command.
func newShowCommand() *cobra.Command {
	options := &platform.QodanaOptions{}
	openDir := false
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a Qodana report",
		Long: `Show (serve) the latest Qodana report. Or open the results directory if the flag is set.

Due to JavaScript security restrictions, the generated report cannot
be viewed via the file:// protocol (by double-clicking the index.html file).
https://www.jetbrains.com/help/qodana/html-report.html
This command serves the Qodana report locally and opens a browser to it.`,
		Run: func(cmd *cobra.Command, args []string) {
			options.FetchAnalyzerSettings()
			if openDir {
				err := core.OpenDir(options.ResultsDir)
				if err != nil {
					log.Fatal(err)
				}
			} else {
				platform.ShowReport(
					options.ResultsDir,
					options.ReportDir,
					options.Port,
				)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.Linter, "linter", "l", "", "Override linter to use")
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVarP(&options.ResultsDir, "results-dir", "o", "", "Override directory to save Qodana inspection results to (default <userCacheDir>/JetBrains/<linter>/results)")
	flags.StringVarP(&options.ReportDir, "report-dir", "r", "", "Override directory to save Qodana HTML report to (default <userCacheDir>/JetBrains/<linter>/results/report)")
	flags.IntVarP(&options.Port, "port", "p", 8080, "Specify port to serve report at")
	flags.BoolVarP(&openDir, "dir-only", "d", false, "Open report directory only, don't serve it")
	flags.StringVar(&options.ConfigName, "config", "", "Set a custom configuration file instead of 'qodana.yaml'. Relative paths in the configuration will be based on the project directory.")
	return cmd
}
