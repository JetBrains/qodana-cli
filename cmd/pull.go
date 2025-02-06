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
	"github.com/JetBrains/qodana-cli/v2024/core"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/startup"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
)

// newPullCommand returns a new instance of the show command.
func newPullCommand() *cobra.Command {
	cliOptions := &pullOptions{}
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull latest version of linter",
		Long:  `An alternative to pull an image.`,
		Run: func(cmd *cobra.Command, args []string) {
			if cliOptions.ConfigName == "" {
				cliOptions.ConfigName = platform.FindDefaultQodanaYaml(cliOptions.ProjectDir)
			}

			startupArgs := startup.ComputeArgs(
				cliOptions.Linter,
				"",
				"",
				"",
				"",
				os.Getenv(platform.QodanaToken),
				os.Getenv(platform.QodanaLicenseOnlyToken),
				false,
				cliOptions.ProjectDir,
				cliOptions.ConfigName,
			)
			if startupArgs.Ide != "" {
				log.Println("Native mode is used, skipping pull")
			} else {
				core.PrepareContainerEnvSettings()
				containerClient, err := client.NewClientWithOpts()
				if err != nil {
					log.Fatal("couldn't connect to container engine ", err)
				}
				core.PullImage(containerClient, startupArgs.Linter)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&cliOptions.Linter, "linter", "l", "", "Override linter to use")
	flags.StringVarP(&cliOptions.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVar(
		&cliOptions.ConfigName,
		"config",
		"",
		"Set a custom configuration file instead of 'qodana.yaml'. Relative paths in the configuration will be based on the project directory.",
	)
	return cmd
}

type pullOptions struct {
	Linter     string
	ProjectDir string
	ConfigName string
}
