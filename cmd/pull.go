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
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// newPullCommand returns a new instance of the show command.
func newPullCommand() *cobra.Command {
	options := &platform.QodanaOptions{}
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull latest version of linter",
		Long:  `An alternative to pull an image.`,
		Run: func(cmd *cobra.Command, args []string) {
			if options.ConfigName == "" {
				options.ConfigName = platform.FindQodanaYaml(options.ProjectDir)
			}
			options.FetchAnalyzerSettings()
			if options.Ide != "" {
				log.Println("Native mode is used, skipping pull")
			} else {
				core.PrepareContainerEnvSettings()
				containerClient, err := client.NewClientWithOpts()
				if err != nil {
					log.Fatal("couldn't connect to container engine ", err)
				}
				core.PullImage(containerClient, options.Linter)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.Linter, "linter", "l", "", "Override linter to use")
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVar(&options.ConfigName, "config", "", "Set a custom configuration file instead of 'qodana.yaml'. Relative paths in the configuration will be based on the project directory.")
	return cmd
}
