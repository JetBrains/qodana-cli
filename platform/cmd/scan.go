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
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
)

// NewScanCommand returns a new instance of the scan command.
func NewScanCommand(options *platform.QodanaOptions) *cobra.Command {
	linterInfo := options.GetLinterSpecificOptions()
	linterName := ""
	if linterInfo == nil {
		log.Fatal("linterInfo is nil")
	} else {
		linterName = (*linterInfo).GetInfo(options).LinterName
	}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan project with Qodana",
		Long: fmt.Sprintf(`Scan a project with Qodana. It runs %s and reports the results.

Note that most options can be configured via qodana.yaml (https://www.jetbrains.com/help/qodana/qodana-yaml.html) file.
But you can always override qodana.yaml options with the following command-line options.
`, linterName),
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetFormatter(&log.TextFormatter{DisableQuote: true, DisableTimestamp: true})
			exitCode, err := platform.RunAnalysis(options)
			if platform.IsContainer() {
				err := platform.ChangePermissionsRecursively(options.ResultsDir)
				if err != nil {
					platform.ErrorMessage("Unable to change permissions in %s: %s", options.ResultsDir, err)
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

	res := platform.ComputeFlags(cmd, options)
	if res != nil {
		log.Fatal("Error while computing flags")
	}

	return cmd
}
