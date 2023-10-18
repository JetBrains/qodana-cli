/*
 * Copyright 2021-2023 JetBrains s.r.o.
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
	"github.com/JetBrains/qodana-cli/v2023/core"
	"github.com/spf13/cobra"
)

// newShowCommand returns a new instance of the show command.
func newSendCommand() *cobra.Command {
	options := &core.QodanaOptions{}
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a Qodana report to Cloud",
		Long: fmt.Sprintf(`Send the report (qodana.sarif.json and other analysis results) to Qodana Cloud. 

If report directory is not specified, the latest report will be fetched from the default linter results location.

If you are using other Qodana Cloud instance than https://qodana.cloud/, override it with declaring %s environment variable.`, core.PrimaryBold(core.QodanaEndpoint)),
		Run: func(cmd *cobra.Command, args []string) {
			core.SendReport(
				options,
				options.ValidateToken(false),
			)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.Linter, "linter", "l", "", "Override linter to use")
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVarP(&options.ResultsDir, "results-dir", "o", options.ResultsDirPath(), "Override directory to save Qodana inspection results to")
	flags.StringVarP(&options.ReportDir, "report-dir", "r", options.ReportDirPath(), "Specify HTML report path (the one with index.html inside) ")
	flags.StringVarP(&options.YamlName, "yaml-name", "y", core.FindQodanaYaml(options.ProjectDir), "Override qodana.yaml name")
	return cmd
}
