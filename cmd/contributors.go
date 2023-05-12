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
	"strconv"

	"github.com/JetBrains/qodana-cli/core"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// showOptions represents show command options.
type contributorsOptions struct {
	ProjectDirs []string
	Days        int
	ExcludeBots bool
}

var pricingUrl = "https://www.jetbrains.com/qodana/buy/"

func getPlanMessage(plan string, cost int, contributors int) string {
	var costMessage string
	if cost == 0 {
		costMessage = fmt.Sprintf("   %s = %d * $0 – Qodana is completely free for %s plan\n",
			core.PrimaryBold("$0"),
			contributors,
			core.PrimaryBold(plan),
		)
	} else {
		costMessage = fmt.Sprintf(
			"   %s = %d * $%d – approximate cost/month for %s plan\n",
			core.PrimaryBold(fmt.Sprintf("$%d", cost*contributors)),
			contributors,
			cost,
			core.PrimaryBold(plan),
		)
	}

	return costMessage
}

// newShowCommand returns a new instance of the show command.
func newContributorsCommand() *cobra.Command {
	options := &contributorsOptions{}
	cmd := &cobra.Command{
		Use:   "contributors",
		Short: "Calculate active project contributors",
		Long: fmt.Sprintf(`
A command-line helper for Qodana pricing[1] to calculate active contributor(s)[2] in the given local repositories.

[1] This pricing is preliminary and subject to change.
Early adopters may receive special offers, which we 
will announce prior to the commercial release.

[2] An active contributor is anyone who has made a commit to any 
of the projects you’ve registered in Qodana Cloud within the last 90 days, 
regardless of when those commits were originally authored. The number of such 
contributors will be calculated using both the commit author information 
and the timestamp for when their contribution to the project was pushed.

[3] Ultimate Plus plan currently has a discount, more information can be found on %s
`, pricingUrl),
		Run: func(cmd *cobra.Command, args []string) {
			if len(options.ProjectDirs) == 0 {
				options.ProjectDirs = append(options.ProjectDirs, ".")
			}
			contributors := core.GetContributors(options.ProjectDirs, options.Days, options.ExcludeBots)
			count := len(contributors)
			contributorsTableData := pterm.TableData{
				{
					core.PrimaryBold("Username"),
					core.PrimaryBold("Email"),
					core.PrimaryBold("Commits"),
				},
			}
			for _, contributor := range contributors {
				contributorsTableData = append(contributorsTableData, []string{
					contributor.Author.Username,
					contributor.Author.Email,
					strconv.Itoa(contributor.Contributions),
				})
			}

			table := pterm.DefaultTable.WithData(contributorsTableData)
			table.HeaderRowSeparator = ""
			table.Separator = " "
			table.Boxed = true
			err := table.Render()
			if err != nil {
				return
			}
			core.EmptyMessage()
			core.SuccessMessage(
				"There are %s active contributor(s)* for the last %s days in the provided %s project(s).",
				core.PrimaryBold(strconv.Itoa(count)),
				core.PrimaryBold(strconv.Itoa(options.Days)),
				core.PrimaryBold(strconv.Itoa(len(options.ProjectDirs))),
			)
			fmt.Print(getPlanMessage("Community", 0, count))
			fmt.Print(getPlanMessage("Ultimate", 6, count))
			fmt.Print(getPlanMessage("Ultimate Plus*", 9, count))
			core.EmptyMessage()
			fmt.Printf(
				`*  Run %s or visit %s for more information.
   Note: Qodana will always be free for verified open source projects.`,
				core.PrimaryBold("qodana contributors -h"),
				pricingUrl,
			)
			core.EmptyMessage()
		},
	}
	flags := cmd.Flags()
	flags.StringArrayVarP(&options.ProjectDirs, "project-dir", "i", []string{}, "Project directory, can be specified multiple times to check multiple projects, if not specified, current directory will be used")
	flags.IntVarP(&options.Days, "days", "d", 30, "Number of days since when to calculate the number of active contributors")
	flags.BoolVar(&options.ExcludeBots, "ignore-bots", true, "Ignore bots (from https://github.com/JetBrains/qodana-cli/blob/main/bots.json) from contributors list")

	return cmd
}
