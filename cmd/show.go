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
	ReportDir string
	Port      int
	OpenDir   bool
}

// NewShowCommand returns a new instance of the show command.
func NewShowCommand() *cobra.Command {
	options := &ShowOptions{}
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show Qodana report",
		Long: `Show (serve locally) the latest Qodana report.

Due to JavaScript security restrictions, the generated report cannot
be viewed via the file:// protocol (by double-clicking the index.html file).
https://www.jetbrains.com/help/qodana/html-report.html
This command serves the Qodana report locally and opens a browser to it.`,
		Run: func(cmd *cobra.Command, args []string) {
			if options.ReportDir == "" {
				linter := core.GetQodanaYaml(".").Linter
				if linter == "" {
					log.Fatalf("Can't automatically find the report...\n" +
						"Please specify the report directory with the --report-dir flag.")
				}
				options.ReportDir = filepath.Join(core.GetLinterSystemDir(".", linter), "results", "report")
			}
			if options.OpenDir {
				err := core.OpenDir(options.ReportDir)
				if err != nil {
					log.Fatal(err)
				}
			} else {
				core.ShowReport(options.ReportDir, options.Port)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ReportDir,
		"report-dir",
		"r",
		"",
		"Specify HTML report path (the one with index.html inside) (default <userCacheDir>/JetBrains/<linter>/results/report)")
	flags.IntVarP(&options.Port, "port", "p", 8080, "Specify port to serve report at")
	flags.BoolVarP(&options.OpenDir, "dir-only", "d", false, "Open report directory only, don't serve it")
	return cmd
}
