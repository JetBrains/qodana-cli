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

package core

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/liamg/clinch/terminal"
	"github.com/mattn/go-isatty"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
)

// Info Two newlines at the start are important to lay the output nicely in CLI.
var Info = fmt.Sprintf(`
  %s (%s)
  https://jb.gg/qodana-cli
  Documentation – https://jb.gg/qodana-docs
  Contact us at qodana-support@jetbrains.com
  Bug Tracker: https://jb.gg/qodana-issue
  Community forum: https://jb.gg/qodana-forum
`, "Qodana CLI", Version)

var PricingUrl = "https://www.jetbrains.com/qodana/buy/"

// IsInteractive returns true if the current execution environment is interactive (useful for colors/animations toggle).
func IsInteractive() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) && os.Getenv("NONINTERACTIVE") == ""
}

// DisableColor disables colors in the output.
func DisableColor() {
	pterm.DisableColor()
}

// styles and different declarations intended to be used only inside this file
var (
	noLineWidth             = 7
	QodanaSpinner           = pterm.DefaultSpinner
	spinnerSequence         = []string{"| ", "/ ", "- ", "\\ "}
	primaryStyle            = pterm.NewStyle()               // primaryStyle is a primary text style.
	primaryBoldStyle        = pterm.NewStyle(pterm.Bold)     // primaryBoldStyle is a primary bold text style.
	errorStyle              = pterm.NewStyle(pterm.FgRed)    // errorStyle is an error style.
	warningStyle            = pterm.NewStyle(pterm.FgYellow) // warningStyle is a warning style.
	miscStyle               = pterm.NewStyle(pterm.FgGray)   // miscStyle is a log style.
	tableSep                = miscStyle.Sprint("─")
	tableSepUp              = miscStyle.Sprint("┬")
	tableSepMid             = miscStyle.Sprint("│")
	tableSepDown            = miscStyle.Sprint("┴")
	tableUp                 = strings.Repeat(tableSep, noLineWidth) + tableSepUp
	tableDown               = strings.Repeat(tableSep, noLineWidth) + tableSepDown
	qodanaInteractiveSelect = pterm.InteractiveSelectPrinter{
		TextStyle:     primaryStyle,
		DefaultText:   "Please select the linter",
		Options:       []string{},
		OptionStyle:   primaryStyle,
		DefaultOption: "",
		MaxHeight:     5,
		Selector:      ">",
		SelectorStyle: primaryStyle,
	}
	DefaultPromptText        = "Do you want to continue?"
	qodanaInteractiveConfirm = pterm.InteractiveConfirmPrinter{
		DefaultValue: true,
		DefaultText:  DefaultPromptText,
		TextStyle:    primaryStyle,
		ConfirmText:  "Yes",
		ConfirmStyle: primaryStyle,
		RejectText:   "No",
		RejectStyle:  primaryStyle,
		SuffixStyle:  primaryStyle,
	}
)

// primary prints a message in the primary style.
func primary(text string, a ...interface{}) string {
	text = fmt.Sprintf(text, a...)
	return primaryStyle.Sprint(text)
}

// PrimaryBold prints a message in the primary bold style.
func PrimaryBold(text string, a ...interface{}) string {
	text = fmt.Sprintf(text, a...)
	return primaryBoldStyle.Sprint(text)
}

// EmptyMessage is a message that is used when there is no message to show.
func EmptyMessage() {
	pterm.Println()
}

// SuccessMessage prints a success message with the icon.
func SuccessMessage(message string, a ...interface{}) {
	message = fmt.Sprintf(message, a...)
	icon := pterm.Green("✓ ")
	pterm.Println(icon, primary(message))
}

// WarningMessage prints a warning message with the icon.
func WarningMessage(message string, a ...interface{}) {
	message = fmt.Sprintf(message, a...)
	icon := warningStyle.Sprint("\n! ")
	pterm.Println(icon, primary(message))
}

// ErrorMessage prints an error message with the icon.
func ErrorMessage(message string, a ...interface{}) {
	message = fmt.Sprintf(message, a...)
	icon := errorStyle.Sprint("✗ ")
	pterm.Println(icon, errorStyle.Sprint(message))
}

// printLinterLog prints the linter logs with color, when needed.
func printLinterLog(line string) {
	if strings.Contains(line, " / /") ||
		strings.Contains(line, "_              _") ||
		strings.Contains(line, "\\/__") ||
		strings.Contains(line, "\\ \\") {
		primaryStyle.Println(line)
	} else {
		miscStyle.Println(line)
	}
}

// printProcess prints the message for processing phase. TODO: Add ETA based on previous runs
func printProcess(f func(), start string, finished string) {
	if err := spin(f, start); err != nil {
		log.Fatal("\nProblem occurred:", err.Error())
	}
	if finished != "" {
		SuccessMessage("Finished %s", finished)
	}
}

// spin creates spinner and runs the given function. Also, spin is a spider in Dutch.
func spin(fun func(), message string) error {
	spinner, _ := startQodanaSpinner(message)
	if spinner == nil {
		fmt.Println(primary(message + "..."))
	}
	fun()
	if spinner != nil {
		spinner.Success()
	}
	return nil
}

// startQodanaSpinner starts a new spinner with the given message.
func startQodanaSpinner(message string) (*pterm.SpinnerPrinter, error) {
	if IsInteractive() {
		QodanaSpinner.Sequence = spinnerSequence
		QodanaSpinner.MessageStyle = primaryStyle
		return QodanaSpinner.WithStyle(pterm.NewStyle(pterm.FgGray)).WithRemoveWhenDone(true).Start(message + "...")
	}
	return nil, nil
}

// updateText updates the text of the spinner.
func updateText(spinner *pterm.SpinnerPrinter, message string) {
	if spinner != nil {
		spinner.UpdateText(message + "...")
	}
}

// PrintFile prints the given file content with lines like printProblem.
func PrintFile(file string) {
	printHeader("", "", file)
	content, err := os.ReadFile(file)
	if err != nil {
		log.Fatalf("failed to read file %s: %s", file, err)
	}
	printLines(string(content), 1, 0, true)
	printFooter("")
}

// printProblem prints problem with source code or without it.
func printProblem(ruleId string, level string, message string, path string, line int, column int, contextLine int, context string) {
	printHeader(level, ruleId, "")
	printPath(path, line, column)
	printLines(context, contextLine, line, false)
	printFooter(message)
}

// getTerminalWidth returns the width of the terminal.
func getTerminalWidth() int {
	width, _ := terminal.Size()
	if width <= 0 {
		width = 80
	}
	return width
}

// printHeader prints the header of the problem/file.
func printHeader(level string, ruleId string, file string) {
	width := getTerminalWidth()
	fmt.Printf("%s %s\n", PrimaryBold(strings.ToUpper(level)), primary(ruleId))
	fmt.Println(strings.Repeat(tableSep, width))
	if file != "" {
		fmt.Printf("%5s  %s %s\n", "", tableSepMid, PrimaryBold(file))
		fmt.Println(strings.Repeat(tableSep, width))
	}
}

// printFooter prints the footer of the problem/file.
func printFooter(message string) {
	fmt.Printf("%s\n", message)
}

// printPath prints the path of the problem.
func printPath(path string, line int, column int) {
	if path != "" && line > 0 && column > 0 {
		fmt.Printf(" %s:%d:%d\n", path, line, column)
		fmt.Printf("%s%s\n", tableUp, strings.Repeat(tableSep, getTerminalWidth()-noLineWidth-1))
	} else {
		fmt.Println(strings.Repeat(tableSep, getTerminalWidth()))
	}
}

// printLines prints the lines of the problem.
func printLines(content string, contextLine int, line int, skipHighlight bool) {
	lines := strings.Split(content, "\n")
	lineCount := len(lines)
	if content[len(content)-1] == '\n' {
		lineCount -= 1 // Remove the last empty line if content ends with a newline
	}
	for i := 0; i < lineCount; i++ {
		var printLine string
		currentLine := contextLine + i
		if skipHighlight {
			printLine = lines[i]
		} else if currentLine == line {
			printLine = errorStyle.Sprint(lines[i]) + " ←"
		} else {
			printLine = warningStyle.Sprint(lines[i])
		}
		lineNumber := miscStyle.Sprintf("%5d", currentLine)
		fmt.Printf("%s  %s %s\n", lineNumber, tableSepMid, printLine)
	}
	fmt.Printf("%s%s\n", tableDown, strings.Repeat(tableSep, getTerminalWidth()-noLineWidth-1))
}

// PrintContributorsTable prints the contributors table and helpful messages.
func PrintContributorsTable(contributors []contributor, days int, dirs int) {
	count := len(contributors)
	contributorsTableData := pterm.TableData{
		{
			PrimaryBold("Username"),
			PrimaryBold("Email"),
			PrimaryBold("Commits"),
		},
	}
	for _, p := range contributors {
		contributorsTableData = append(contributorsTableData, []string{
			p.Author.Username,
			p.Author.Email,
			strconv.Itoa(p.Contributions),
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
	EmptyMessage()
	SuccessMessage(
		"There are %s active contributor(s)* for the last %s days in the provided %s project(s).",
		PrimaryBold(strconv.Itoa(count)),
		PrimaryBold(strconv.Itoa(days)),
		PrimaryBold(strconv.Itoa(dirs)),
	)
	fmt.Print(getPlanMessage("Community", 0, count))
	fmt.Print(getPlanMessage("Ultimate", 6, count))
	fmt.Print(getPlanMessage("Ultimate Plus*", 9, count))
	EmptyMessage()
	fmt.Printf(
		`*  Run %s or visit %s for more information.
   Note: Qodana will always be free for verified open source projects.`,
		PrimaryBold("qodana contributors -h"),
		PricingUrl,
	)
	EmptyMessage()
}

// getPlanMessage returns a message with the cost of the plan.
func getPlanMessage(plan string, cost int, contributors int) string {
	var costMessage string
	if cost == 0 {
		costMessage = fmt.Sprintf("   %s = %d * $0 – Qodana is completely free for %s plan\n",
			PrimaryBold("$0"),
			contributors,
			PrimaryBold(plan),
		)
	} else {
		costMessage = fmt.Sprintf(
			"   %s = %d * $%d – approximate cost/month for %s plan\n",
			PrimaryBold(fmt.Sprintf("$%d", cost*contributors)),
			contributors,
			cost,
			PrimaryBold(plan),
		)
	}

	return costMessage
}
