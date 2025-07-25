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

package platform

import (
	"fmt"
	"os"

	platformcmd "github.com/JetBrains/qodana-cli/v2025/platform/cmd"
	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewThirdPartyScanCommand returns a new instance of the scan command.
func NewThirdPartyScanCommand(linter ThirdPartyLinter, linterInfo thirdpartyscan.LinterInfo) *cobra.Command {
	cliOptions := &platformcmd.CliOptions{}
	c := &cobra.Command{
		Use:   "scan",
		Short: "Scan project with Qodana",
		Long: fmt.Sprintf(
			`Scan a project with Qodana. It runs %s and reports the results.

Note that most options can be configured via qodana.yaml (https://www.jetbrains.com/help/qodana/qodana-yaml.html) file.
But you can always override qodana.yaml options with the following command-line options.
`, linterInfo.LinterPresentableName,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetFormatter(&log.TextFormatter{DisableQuote: true, DisableTimestamp: true})
			exitCode, err := RunThirdPartyLinterAnalysis(*cliOptions, linter, linterInfo)

			log.Debug("exitCode: ", exitCode)
			if exitCode == utils.QodanaFailThresholdExitCode {
				msg.EmptyMessage()
				msg.ErrorMessage("The number of problems exceeds the fail threshold")
				os.Exit(exitCode)
			}
			return err
		},
	}

	err := platformcmd.ComputeFlags(c, cliOptions)
	if err != nil {
		log.Fatal("Error while computing flags")
	}
	if cliOptions.Linter != "" {
		msg.WarningMessage("Warning: --linter option is ignored when running a third-party linter.")
	}
	if cliOptions.Ide != "" {
		msg.WarningMessage("Warning: --ide option is ignored when running a third-party linter.")
	}
	if cliOptions.Image != "" {
		msg.WarningMessage("Warning: --image option is ignored when running a third-party linter.")
	}
	if cliOptions.WithinDocker != "" {
		msg.WarningMessage("Warning: --within-docker option is ignored when running a third-party linter.")
	}

	return c
}
