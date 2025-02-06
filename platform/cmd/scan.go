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

package platformcmd

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/cli"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
)

// NewScanCommand returns a new instance of the scan command.
func NewScanCommand(linter platform.ThirdPartyLinter, linterInfo platform.LinterInfo) *cobra.Command {
	cliOptions := &cli.QodanaScanCliOptions{}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan project with Qodana",
		Long: fmt.Sprintf(
			`Scan a project with Qodana. It runs %s and reports the results.

Note that most options can be configured via qodana.yaml (https://www.jetbrains.com/help/qodana/qodana-yaml.html) file.
But you can always override qodana.yaml options with the following command-line options.
`, linterInfo,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetFormatter(&log.TextFormatter{DisableQuote: true, DisableTimestamp: true})
			c, exitCode, err := platform.RunThirdPartyLinterAnalysis(*cliOptions, linter, linterInfo)

			resultDir := c.ResultsDir()
			if resultDir == "" {
				resultDir = cliOptions.ResultsDir
			}
			if platform.IsContainer() {
				c.ResultsDir()
				err := platform.ChangePermissionsRecursively(resultDir)
				if err != nil {
					platform.ErrorMessage("Unable to change permissions in %s: %s", resultDir, err)
				}
			}
			log.Debug("exitCode: ", exitCode)
			if exitCode == platform.QodanaFailThresholdExitCode {
				platform.EmptyMessage()
				platform.ErrorMessage("The number of problems exceeds the fail threshold")
				os.Exit(exitCode)
			}
			return err
		},
	}

	err := cli.ComputeFlags(cmd, cliOptions)
	if err != nil {
		log.Fatal("Error while computing flags")
	}

	return cmd
}
