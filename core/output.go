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

package core

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	"github.com/pterm/pterm"
	"strconv"
)

var PricingUrl = "https://www.jetbrains.com/qodana/buy/"

// PrintContributorsTable prints the contributors table and helpful messages.
func PrintContributorsTable(contributors []contributor, days int, dirs int) {
	count := len(contributors)
	contributorsTableData := pterm.TableData{
		[]string{
			msg.PrimaryBold("Username"),
			msg.PrimaryBold("Email"),
			msg.PrimaryBold("Commits"),
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
	msg.EmptyMessage()
	msg.SuccessMessage(
		"There are %s active contributor(s)* for the last %s days in the provided %s project(s).",
		msg.PrimaryBold(strconv.Itoa(count)),
		msg.PrimaryBold(strconv.Itoa(days)),
		msg.PrimaryBold(strconv.Itoa(dirs)),
	)
	msg.EmptyMessage()
	fmt.Printf(
		`*  Visit %s for more information.
   Note: Qodana will always be free for verified open source projects.`,
		PricingUrl,
	)
	msg.EmptyMessage()
}
