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
	"strconv"

	"github.com/JetBrains/qodana-cli/core"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// showOptions represents show command options.
type contributorsOptions struct {
	ProjectDir  string
	Days        int
	ExcludeBots bool
}

var pricingUrl = "https://www.jetbrains.com/qodana/buy/"

func getPlanMessage(plan string, cost int, contributors int) string {
	return fmt.Sprintf(
		"   %s = %d * $%d – approximate cost/month for %s plan\n",
		core.PrimaryBold(fmt.Sprintf("$%d", cost*contributors)),
		contributors,
		cost,
		core.PrimaryBold(plan),
	)
}

// newShowCommand returns a new instance of the show command.
func newContributorsCommand() *cobra.Command {
	options := &contributorsOptions{}
	cmd := &cobra.Command{
		Use:   "contributors",
		Short: "Calculate active project contributors",
		Long: fmt.Sprintf(`A command-line helper for Qodana pricing to calculate active contributors* in the given repository.

* An active contributor is anyone who has made a commit to any 
of the projects you’ve registered in Qodana Cloud within the last 90 days, 
regardless of when those commits were originally authored. The number of such 
contributors will be calculated using both the commit author information 
and the timestamp for when their contribution to the project was pushed.

** Ultimate Plus plan currently has a discount, more information can be found on %s
`, pricingUrl),
		Run: func(cmd *cobra.Command, args []string) {
			contributors := core.GetContributors(options.ProjectDir, options.Days, options.ExcludeBots)

			count := strconv.Itoa(len(contributors))
			core.EmptyMessage()
			contributorsTableData := pterm.TableData{
				{core.PrimaryBold("Username"), core.PrimaryBold("Email"), core.PrimaryBold("Commits")},
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
				"There are %s active contributors* for the last %s days",
				core.PrimaryBold(count),
				core.PrimaryBold(strconv.Itoa(options.Days)),
			)
			fmt.Print(getPlanMessage("Ultimate", 6, len(contributors)))
			fmt.Print(getPlanMessage("Ultimate Plus*", 9, len(contributors)))
			core.EmptyMessage()
			fmt.Printf(
				`*  Run %s or visit %s for more information.`,
				core.PrimaryBold("qodana contributors -h"),
				pricingUrl,
			)
			core.EmptyMessage()
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.IntVarP(&options.Days, "days", "d", 30, "Number of days since when to calculate the number of active contributors")
	flags.BoolVar(&options.ExcludeBots, "ignore-bots", true, "Ignore bots (from https://github.com/JetBrains/qodana-cli/blob/main/bots.json) from contributors list")
	return cmd
}
