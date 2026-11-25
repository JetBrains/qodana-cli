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
	"errors"

	"github.com/JetBrains/qodana-cli/internal/core"
	"github.com/JetBrains/qodana-cli/internal/platform"
	"github.com/JetBrains/qodana-cli/internal/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/JetBrains/qodana-cli/internal/platform/telemetry"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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
			platform.StartStatistics("show", cliOptions.ProjectDir, cliOptions.NoStatistics, nil)
			qdenv.InitializeQodanaGlobalEnv(qdenv.EmptyEnvProvider())

			commonCtx := commoncontext.Compute(
				cliOptions.Linter,
				"",
				"",
				"",
				"",
				cliOptions.ResultsDir,
				cliOptions.ReportDir,
				qdenv.GetQodanaGlobalEnv(qdenv.QodanaToken),
				false,
				cliOptions.ProjectDir,
				"",
				cliOptions.ConfigName,
			)
			if cliOptions.OpenDir {
				err := core.OpenDir(commonCtx.ResultsDir)
				if err != nil {
					log.Fatal(err)
				}
				telemetry.ReportShown("dir")
			} else {
				commoncontext.ShowReport(
					commonCtx.CacheDir,
					commonCtx.ResultsDir,
					commonCtx.ReportDir,
					cliOptions.Port,
					reportShown,
				)
			}
			telemetry.Finish(0, "")
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
	flags.BoolVar(&cliOptions.NoStatistics, "no-statistics", false, "Disable sending anonymous statistics")
	return cmd
}

// reportShown records where commoncontext.ShowReport shows the report, it is shared with scans: they don't record it. A
// local report is served until the user interrupts qodana show, which is no failure: the run is finished meanwhile. A
// report that can't be shown sets the reason the run finishes with: a missing one fails it, a port in use only warns.
func reportShown(target string, err error) {
	if reason := showFailure(err); reason != "" {
		telemetry.Fail(reason)
		return
	}
	telemetry.ReportShown(target)
	if target == "local" {
		go telemetry.Finish(0, "")
	}
}

// showFailure gives the reason of a report that can't be shown, "" for a shown one.
func showFailure(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, commoncontext.ErrReportMissing):
		return telemetry.ReasonReportMissing
	default:
		return telemetry.ReasonServeFailed
	}
}

type showOptions struct {
	Linter       string
	ProjectDir   string
	ResultsDir   string
	ReportDir    string
	Port         int
	OpenDir      bool
	ConfigName   string
	NoStatistics bool
}
