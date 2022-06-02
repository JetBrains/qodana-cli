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
	"path/filepath"

	"github.com/JetBrains/qodana-cli/core"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// ShowOptions represents scan command options.
type ShowOptions struct {
	ProjectDir string
	ResultsDir string
	ReportDir  string
	Port       int
	OpenDir    bool
	YamlName   string
}

// NewShowCommand returns a new instance of the show command.
func NewShowCommand() *cobra.Command {
	options := &ShowOptions{}
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a Qodana report",
		Long: `Show (serve) the latest Qodana report. Or open the results directory if the flag is set.

Due to JavaScript security restrictions, the generated report cannot
be viewed via the file:// protocol (by double-clicking the index.html file).
https://www.jetbrains.com/help/qodana/html-report.html
This command serves the Qodana report locally and opens a browser to it.`,
		Run: func(cmd *cobra.Command, args []string) {
			if options.YamlName == "" {
				options.YamlName = core.FindQodanaYaml(options.ProjectDir)
			}
			if options.ReportDir == "" {
				linter := core.LoadQodanaYaml(options.ProjectDir, options.YamlName).Linter
				if linter == "" {
					log.Fatalf("Can't automatically find the report...\n" +
						"Please specify the report directory with the --report-dir flag.")
				}
				options.ResultsDir = filepath.Join(core.GetLinterSystemDir(options.ProjectDir, linter), "results")
				options.ReportDir = filepath.Join(options.ResultsDir, "report")
			}
			if options.OpenDir {
				err := core.OpenDir(options.ResultsDir)
				if err != nil {
					log.Fatal(err)
				}
			} else {
				core.ShowReport(options.ReportDir, options.Port)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVarP(&options.ReportDir,
		"report-dir",
		"r",
		"",
		"Specify HTML report path (the one with index.html inside) (default <userCacheDir>/JetBrains/<linter>/results/report)")
	flags.IntVarP(&options.Port, "port", "p", 8080, "Specify port to serve report at")
	flags.BoolVarP(&options.OpenDir, "dir-only", "d", false, "Open report directory only, don't serve it")
	flags.StringVarP(&options.YamlName, "yaml-name", "y", "", "Override qodana.yaml name")
	return cmd
}
