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
	"github.com/JetBrains/qodana-cli/v2025/platform"
	"github.com/JetBrains/qodana-cli/v2025/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2025/platform/tokenloader"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"os"
)

// newShowCommand returns a new instance of the show command.
func newSendCommand() *cobra.Command {
	cliOptions := &sendOptions{}
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a Qodana report to Cloud",
		Long: fmt.Sprintf(
			`Send the report (qodana.sarif.json and other analysis results) to Qodana Cloud. 

If report directory is not specified, the latest report will be fetched from the default linter results location.

If you are using other Qodana Cloud instance than https://qodana.cloud/, override it by declaring the %s environment variable.`,
			msg.PrimaryBold(qdenv.QodanaEndpointEnv),
		),
		Run: func(cmd *cobra.Command, args []string) {
			commonCtx := commoncontext.Compute(
				cliOptions.Linter,
				"",
				"",
				cliOptions.ResultsDir,
				cliOptions.ReportDir,
				os.Getenv(qdenv.QodanaToken),
				false,
				cliOptions.ProjectDir,
				cliOptions.ConfigName,
			)

			publisher := platform.Publisher{
				ResultsDir: commonCtx.ResultsDir,
				LogDir:     commonCtx.LogDir(),
				AnalysisId: cliOptions.AnalysisId,
			}

			java := ""
			if utils.IsInstalled("java") {
				java = "java"
			}
			platform.SendReport(
				publisher,
				tokenloader.ValidateCloudToken(commonCtx, false),
				java,
			)
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
	flags.StringVar(
		&cliOptions.ConfigName,
		"config",
		"",
		"Set a custom configuration file instead of 'qodana.yaml'. Relative paths in the configuration will be based on the project directory.",
	)
	flags.StringVarP(
		&cliOptions.AnalysisId,
		"analysis-id",
		"a",
		uuid.New().String(),
		"Unique report identifier (GUID) to be used by Qodana Cloud",
	)
	return cmd
}

type sendOptions struct {
	Linter     string
	ProjectDir string
	ResultsDir string
	ReportDir  string
	ConfigName string
	AnalysisId string
}
