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

	"github.com/JetBrains/qodana-cli/v2025/core"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// contributorsOptions represents contributor command options.
type contributorsOptions struct {
	ProjectDirs []string
	Days        int
	Output      string
}

// newShowCommand returns a new instance of the show command.
func newContributorsCommand() *cobra.Command {
	options := &contributorsOptions{}
	cmd := &cobra.Command{
		Use:   "contributors",
		Short: "Calculate active project contributors",
		Long: fmt.Sprintf(
			`
A command-line helper for Qodana pricing[1] to calculate active contributor(s)[2] in the given local repositories.

[1] More information about available Qodana plans can be found at %s
`, core.PricingUrl,
		),
		Run: func(cmd *cobra.Command, args []string) {
			if len(options.ProjectDirs) == 0 {
				options.ProjectDirs = append(options.ProjectDirs, ".")
			}
			contributors := core.GetContributors(options.ProjectDirs, options.Days, false)
			switch options.Output {
			case "tabular":
				core.PrintContributorsTable(contributors, options.Days, len(options.ProjectDirs))
				return
			case "json":
				out, err := core.ToJSON(contributors)
				if err != nil {
					log.Fatalf("Failed to convert to JSON: %s", err)
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
				if err != nil {
					log.Fatalf("Failed to write to stdout: %s", err)
				}
				return
			default:
				log.Fatalf("Unknown output format: %s", options.Output)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringArrayVarP(
		&options.ProjectDirs,
		"project-dir",
		"i",
		[]string{},
		"Project directory, can be specified multiple times to check multiple projects, if not specified, current directory will be used",
	)
	flags.IntVarP(
		&options.Days,
		"days",
		"d",
		90,
		"Number of days since when to calculate the number of active contributors",
	)
	flags.StringVarP(&options.Output, "output", "o", "tabular", "Output format, can be tabular or json")

	return cmd
}
