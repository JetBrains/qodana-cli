package core

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2023/platform"
	"github.com/pterm/pterm"
	"strconv"
)

var PricingUrl = "https://www.jetbrains.com/qodana/buy/"

// PrintContributorsTable prints the contributors table and helpful messages.
func PrintContributorsTable(contributors []contributor, days int, dirs int) {
	count := len(contributors)
	contributorsTableData := pterm.TableData{
		[]string{
			platform.PrimaryBold("Username"),
			platform.PrimaryBold("Email"),
			platform.PrimaryBold("Commits"),
		},
	}
	for _, p := range contributors {
		contributorsTableData = append(contributorsTableData, []string{
			p.Author.Username,
			p.Author.Email,
			strconv.Itoa(p.Count),
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
	platform.EmptyMessage()
	platform.SuccessMessage(
		"There are %s active contributor(s)* for the last %s days in the provided %s project(s).",
		platform.PrimaryBold(strconv.Itoa(count)),
		platform.PrimaryBold(strconv.Itoa(days)),
		platform.PrimaryBold(strconv.Itoa(dirs)),
	)
	fmt.Print(getPlanMessage("Community", 0, count))
	fmt.Print(getPlanMessage("Ultimate", 6, count))
	fmt.Print(getPlanMessage("Ultimate Plus*", 9, count))
	platform.EmptyMessage()
	fmt.Printf(
		`*  Run %s or visit %s for more information.
   Note: Qodana will always be free for verified open source projects.`,
		platform.PrimaryBold("qodana contributors -h"),
		PricingUrl,
	)
	platform.EmptyMessage()
}

// getPlanMessage returns a message with the cost of the plan.
func getPlanMessage(plan string, cost int, contributors int) string {
	var costMessage string
	if cost == 0 {
		costMessage = fmt.Sprintf("   %s = %d * $0 – Qodana is completely free for %s plan\n",
			platform.PrimaryBold("$0"),
			contributors,
			platform.PrimaryBold(plan),
		)
	} else {
		costMessage = fmt.Sprintf(
			"   %s = %d * $%d – approximate cost/month for %s plan\n",
			platform.PrimaryBold(fmt.Sprintf("$%d", cost*contributors)),
			contributors,
			cost,
			platform.PrimaryBold(plan),
		)
	}

	return costMessage
}
