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
	"github.com/JetBrains/qodana-cli/v2024/preparehost/product"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

// newInitCommand returns a new instance of the show command.
func newInitCommand() *cobra.Command {
	options := &platform.QodanaOptions{}
	force := false
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Configure a project for Qodana",
		Long:  `Configure a project for Qodana: prepare Qodana configuration file by analyzing the project structure and generating a default configuration qodana.yaml file.`,
		Run: func(cmd *cobra.Command, args []string) {
			emptyProduct := product.Product{} // TODO what to do with product?

			qodanaYaml := platform.LoadQodanaYaml(options.ProjectDir, options.ConfigName)
			if (qodanaYaml.Linter == "" && qodanaYaml.Ide == "") || force {
				absPath, err := filepath.Abs(options.ProjectDir)
				if err != nil {
					log.Fatal(err)
				}
				options.ProjectDir = absPath
				if platform.IsInteractive() && !platform.AskUserConfirm(fmt.Sprintf("Do you want to set up Qodana in %s", platform.PrimaryBold(options.ProjectDir))) {
					return
				}
				token := os.Getenv(platform.QodanaToken)
				options.Setenv(platform.QodanaToken, token)
				analyzer := platform.GetAnalyzer(options.ProjectDir, token)
				platform.WriteQodanaLinterToYamlFile(options.ProjectDir, analyzer, options.CoverageDir)
				if platform.IsNativeAnalyzer(analyzer) {
					options.Ide = analyzer
				} else {
					options.Linter = analyzer
				}
			} else {
				platform.EmptyMessage()
				var analyzer string
				if qodanaYaml.Ide != "" {
					analyzer = qodanaYaml.Ide
				} else if qodanaYaml.Linter != "" {
					analyzer = qodanaYaml.Linter
				}
				platform.SuccessMessage(
					"The product to use was already configured before: %s. Run the command with %s flag to re-init the project",
					platform.PrimaryBold(analyzer),
					platform.PrimaryBold("-f"),
				)
			}
			if platform.IsInteractive() && qodanaYaml.IsDotNet() && (qodanaYaml.DotNet.IsEmpty() || force) {
				if platform.GetDotNetConfig(options.ProjectDir, options.ConfigName) {
					platform.SuccessMessage("The .NET configuration was successfully set")
				}
			}
			platform.PrintFile(filepath.Join(options.ProjectDir, options.ConfigName))
			options.Linter = qodanaYaml.Linter
			options.Ide = qodanaYaml.Ide
			if options.RequiresToken(emptyProduct.IsEap || emptyProduct.IsCommunity()) {
				options.ValidateToken(force)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the project to configure")
	flags.BoolVarP(&force, "force", "f", false, "Force initialization (overwrite existing valid qodana.yaml)")
	flags.StringVar(&options.ConfigName, "config", "", "Set a custom configuration file instead of 'qodana.yaml'. Relative paths in the configuration will be based on the project directory.")
	return cmd
}
