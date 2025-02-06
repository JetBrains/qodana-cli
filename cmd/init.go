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
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/product"
	"github.com/JetBrains/qodana-cli/v2024/platform/startup"
	"github.com/JetBrains/qodana-cli/v2024/platform/tokenloader"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

// newInitCommand returns a new instance of the show command.
func newInitCommand() *cobra.Command {
	cliOptions := &initOptions{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Configure a project for Qodana",
		Long:  `Configure a project for Qodana: prepare Qodana configuration file by analyzing the project structure and generating a default configuration qodana.yaml file.`,
		Run: func(cmd *cobra.Command, args []string) {
			emptyProduct := product.Product{} // TODO what to do with product?

			qodanaYaml := platform.LoadQodanaYaml(cliOptions.ProjectDir, cliOptions.ConfigName)

			ide := qodanaYaml.Ide
			linter := qodanaYaml.Linter
			if (linter == "" && ide == "") || cliOptions.Force {
				absPath, err := filepath.Abs(cliOptions.ProjectDir)
				if err != nil {
					log.Fatal(err)
				}
				cliOptions.ProjectDir = absPath
				if platform.IsInteractive() && !platform.AskUserConfirm(
					fmt.Sprintf(
						"Do you want to set up Qodana in %s",
						platform.PrimaryBold(cliOptions.ProjectDir),
					),
				) {
					return
				}
				token := os.Getenv(platform.QodanaToken)
				analyzer := platform.GetAnalyzer(cliOptions.ProjectDir, token)

				platform.WriteQodanaLinterToYamlFile(cliOptions.ProjectDir, analyzer, cliOptions.ConfigName)
				if platform.IsNativeAnalyzer(analyzer) {
					ide = analyzer
				} else {
					linter = analyzer
				}
			} else {
				platform.EmptyMessage()
				var analyzer string
				if ide != "" {
					analyzer = ide
				} else if linter != "" {
					analyzer = linter
				}
				platform.SuccessMessage(
					"The product to use was already configured before: %s. Run the command with %s flag to re-init the project",
					platform.PrimaryBold(analyzer),
					platform.PrimaryBold("-f"),
				)
			}
			if platform.IsInteractive() && qodanaYaml.IsDotNet() && (qodanaYaml.DotNet.IsEmpty() || cliOptions.Force) {
				if platform.GetAndSaveDotNetConfig(cliOptions.ProjectDir, cliOptions.ConfigName) {
					platform.SuccessMessage("The .NET configuration was successfully set")
				}
			}
			platform.PrintFile(filepath.Join(cliOptions.ProjectDir, cliOptions.ConfigName))

			startupArgs := startup.ComputeArgs(
				linter,
				ide,
				"",
				"",
				"",
				os.Getenv(platform.QodanaToken),
				os.Getenv(platform.QodanaLicenseOnlyToken),
				false,
				cliOptions.ProjectDir,
				cliOptions.ConfigName,
			)
			if tokenloader.IsCloudTokenRequired(startupArgs, emptyProduct.IsEap || emptyProduct.IsCommunity()) {
				tokenloader.ValidateToken(startupArgs, cliOptions.Force)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&cliOptions.ProjectDir, "project-dir", "i", ".", "Root directory of the project to configure")
	flags.BoolVarP(
		&cliOptions.Force,
		"force",
		"f",
		false,
		"Force initialization (overwrite existing valid qodana.yaml)",
	)
	flags.StringVar(
		&cliOptions.ConfigName,
		"config",
		"",
		"Set a custom configuration file instead of 'qodana.yaml'. Relative paths in the configuration will be based on the project directory.",
	)
	return cmd
}

type initOptions struct {
	ProjectDir string
	ConfigName string
	Force      bool
}
