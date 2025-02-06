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
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	"github.com/JetBrains/qodana-cli/v2024/platform/platforminit"
	"github.com/JetBrains/qodana-cli/v2024/platform/product"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdyaml"
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

			qodanaYaml := qdyaml.LoadQodanaYaml(cliOptions.ProjectDir, cliOptions.ConfigName)

			ide := qodanaYaml.Ide
			linter := qodanaYaml.Linter
			if (linter == "" && ide == "") || cliOptions.Force {
				absPath, err := filepath.Abs(cliOptions.ProjectDir)
				if err != nil {
					log.Fatal(err)
				}
				cliOptions.ProjectDir = absPath
				if msg.IsInteractive() && !msg.AskUserConfirm(
					fmt.Sprintf(
						"Do you want to set up Qodana in %s",
						msg.PrimaryBold(cliOptions.ProjectDir),
					),
				) {
					return
				}
				token := os.Getenv(qdenv.QodanaToken)
				analyzer := platforminit.GetAnalyzer(cliOptions.ProjectDir, token)

				qdyaml.WriteQodanaLinterToYamlFile(
					cliOptions.ProjectDir,
					analyzer,
					cliOptions.ConfigName,
					product.AllCodes,
				)
				if product.IsNativeAnalyzer(analyzer) {
					ide = analyzer
				} else {
					linter = analyzer
				}
			} else {
				msg.EmptyMessage()
				var analyzer string
				if ide != "" {
					analyzer = ide
				} else if linter != "" {
					analyzer = linter
				}
				msg.SuccessMessage(
					"The product to use was already configured before: %s. Run the command with %s flag to re-init the project",
					msg.PrimaryBold(analyzer),
					msg.PrimaryBold("-f"),
				)
			}
			if msg.IsInteractive() && qodanaYaml.IsDotNet() && (qodanaYaml.DotNet.IsEmpty() || cliOptions.Force) {
				if platforminit.GetAndSaveDotNetConfig(cliOptions.ProjectDir, cliOptions.ConfigName) {
					msg.SuccessMessage("The .NET configuration was successfully set")
				}
			}
			msg.PrintFile(filepath.Join(cliOptions.ProjectDir, cliOptions.ConfigName))

			startupArgs := platforminit.ComputeArgs(
				linter,
				ide,
				"",
				"",
				"",
				os.Getenv(qdenv.QodanaToken),
				os.Getenv(qdenv.QodanaLicenseOnlyToken),
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
