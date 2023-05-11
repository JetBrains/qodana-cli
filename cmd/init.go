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
	"fmt"
	"path/filepath"
	"strings"

	"github.com/JetBrains/qodana-cli/core"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// initOptions represents init command options.
type initOptions struct {
	ProjectDir string
	Force      bool
	YamlName   string
}

// newInitCommand returns a new instance of the show command.
func newInitCommand() *cobra.Command {
	options := &initOptions{}
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
				absPath, err := filepath.Abs(options.ProjectDir)
				if err != nil {
					log.Fatal(err)
				}
				if core.IsInteractive() && !core.AskUserConfirm(fmt.Sprintf("Do you want to set up Qodana in %s", absPath)) {
					return
				}
				qodanaYaml.Linter = core.GetLinter(options.ProjectDir, options.YamlName)
			} else {
				latestLinter := core.GetLatestVersion(qodanaYaml.Linter)
				if latestLinter != qodanaYaml.Linter {
					core.WarningMessage("You are using an outdated %s linter\n", qodanaYaml.Linter)
					if core.AskUserConfirm(
						fmt.Sprintf("Do you want to update to %s", latestLinter),
					) {
						core.SetQodanaLinter(options.ProjectDir, latestLinter, options.YamlName)
						qodanaYaml.Linter = latestLinter
					}
				} else {
					core.EmptyMessage()
					core.SuccessMessage(
						"The linter was already configured before: %s. Run the command with %s flag to re-init the project",
						core.PrimaryBold(qodanaYaml.Linter),
						core.PrimaryBold("-f"),
					)
				}
			}
			if core.IsInteractive() && strings.Contains(qodanaYaml.Linter, "dotnet") && (qodanaYaml.DotNet.IsEmpty() || options.Force) {
				if core.GetDotNetConfig(options.ProjectDir, options.YamlName) {
					core.SuccessMessage("The .NET configuration was successfully set")
				}
			}
			core.PrintFile(filepath.Join(options.ProjectDir, options.YamlName))
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the project to configure")
	flags.BoolVarP(&options.Force, "force", "f", false, "Force initialization (overwrite existing valid qodana.yaml)")
	flags.StringVar(&options.YamlName, "yaml-name", "", "Override qodana.yaml name")
	return cmd
}
