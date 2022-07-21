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
	"github.com/JetBrains/qodana-cli/core"
	"github.com/spf13/cobra"
)

// InitOptions represents scan command options.
type InitOptions struct {
	ProjectDir string
	Force      bool
	YamlName   string
}

// NewInitCommand returns a new instance of the show command.
func NewInitCommand() *cobra.Command {
	options := &InitOptions{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Configure a project for Qodana",
		Long:  `Configure a project for Qodana: prepare Qodana configuration file by analyzing the project structure and generating a default configuration qodana.yaml file.`,
		Run: func(cmd *cobra.Command, args []string) {
			if options.YamlName == "" {
				options.YamlName = core.FindQodanaYaml(options.ProjectDir)
			}
			qodanaYaml := core.LoadQodanaYaml(options.ProjectDir, options.YamlName)
			if qodanaYaml.Linter == "" || options.Force {
				core.GetLinter(options.ProjectDir, options.YamlName)
			} else {
				core.EmptyMessage()
				core.SuccessMessage(
					"The linter was already configured before: %s. Run the command with %s flag to re-init the project",
					core.PrimaryBold(qodanaYaml.Linter),
					core.PrimaryBold("-f"),
				)
			}
			core.WarningMessage("Run %s to analyze the project. The configuration is stored in %s and can be changed later", core.PrimaryBold("qodana scan"), core.PrimaryBold(options.YamlName))
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the project to configure")
	flags.BoolVarP(&options.Force, "force", "f", false, "Force initialization (overwrite existing valid qodana.yaml)")
	flags.StringVar(&options.YamlName, "yaml-name", "", "Override qodana.yaml name")
	return cmd
}
