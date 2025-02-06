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

package msg

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	cienvironment "github.com/cucumber/ci-environment/go"
	"os"
	"strings"

	"github.com/liamg/clinch/terminal"
	"github.com/mattn/go-isatty"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
)

var QodanaInteractiveSelect = pterm.InteractiveSelectPrinter{
	TextStyle:     PrimaryStyle,
	DefaultText:   "Please select the product to use",
	Options:       []string{},
	OptionStyle:   PrimaryStyle,
	DefaultOption: "",
	MaxHeight:     5,
	Selector:      ">",
	SelectorStyle: PrimaryStyle,
}

// InfoString Two newlines at the start are important to lay the output nicely in CLI.
func InfoString(version string) string {
	return fmt.Sprintf(
		`
  %s (%s)
  https://jb.gg/qodana-cli
  Documentation – https://jb.gg/qodana-docs
  Contact us at qodana-support@jetbrains.com
  Bug Tracker: https://jb.gg/qodana-issue
  Community forum: https://jb.gg/qodana-forum
`, "Qodana CLI", version,
	)
}

// IsInteractive returns true if the current execution environment is interactive (useful for colors/animations toggle).
func IsInteractive() bool {
	return !qdenv.IsContainer() && os.Getenv("NONINTERACTIVE") == "" && (isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()))
}

// DisableColor disables colors in the output.
func DisableColor() {
	pterm.DisableColor()
}

// styles and different declarations intended to be used only inside this file
var (
	noLineWidth       = 7
	QodanaSpinner     = pterm.DefaultSpinner
	spinnerSequence   = []string{"| ", "/ ", "- ", "\\ "}
	PrimaryStyle      = pterm.NewStyle()               // PrimaryStyle is a primary text style.
	primaryBoldStyle  = pterm.NewStyle(pterm.Bold)     // primaryBoldStyle is a Primary bold text style.
	errorStyle        = pterm.NewStyle(pterm.FgRed)    // errorStyle is an error style.
	warningStyle      = pterm.NewStyle(pterm.FgYellow) // warningStyle is a warning style.
	miscStyle         = pterm.NewStyle(pterm.FgGray)   // miscStyle is a log style.
	tableSep          = miscStyle.Sprint("─")
	tableSepUp        = miscStyle.Sprint("┬")
	tableSepMid       = miscStyle.Sprint("│")
	tableSepDown      = miscStyle.Sprint("┴")
	tableUp           = strings.Repeat(tableSep, noLineWidth) + tableSepUp
	tableDown         = strings.Repeat(tableSep, noLineWidth) + tableSepDown
	DefaultPromptText = "Do you want to continue?"
)

// Primary prints a message in the Primary style.
func Primary(text string, a ...interface{}) string {
	text = fmt.Sprintf(text, a...)
	return PrimaryStyle.Sprint(text)
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
	pterm.Println(icon, Primary(message))
}

// WarningMessage prints a warning message with the icon.
func WarningMessage(message string, a ...interface{}) {
	message = fmt.Sprintf(message, a...)
	icon := warningStyle.Sprint("\n! ")
	pterm.Println(icon, Primary(message))
}

// WarningMessageCI prints a warning message to the CI environment (additional highlighting).
func WarningMessageCI(message string, a ...interface{}) {
	message = fmt.Sprintf(message, a...)
	pterm.Println(formatMessageForCI("warning", message))
}

// ErrorMessage prints an error message with the icon.
func ErrorMessage(message string, a ...interface{}) {
	message = fmt.Sprintf(message, a...)
	icon := errorStyle.Sprint("✗ ")
	pterm.Println(icon, errorStyle.Sprint(message))
}

// PrintLinterLog prints the linter logs with color, when needed.
func PrintLinterLog(line string) {
	if strings.Contains(line, " / /") ||
		strings.Contains(line, "_              _") ||
		strings.Contains(line, "\\/__") ||
		strings.Contains(line, "\\ \\") {
		PrimaryStyle.Println(line)
	} else {
		miscStyle.Println(line)
	}
}

// PrintProcess prints the message for processing phase. TODO: Add ETA based on previous runs
func PrintProcess(f func(spinner *pterm.SpinnerPrinter), start string, finished string) {
	if err := spin(f, start); err != nil {
		log.Fatal("\nProblem occurred:", err.Error())
	}
	if finished != "" {
		SuccessMessage("Finished %s", finished)
	}
}

// spin creates spinner and runs the given function. Also, spin is a spider in Dutch.
func spin(fun func(spinner *pterm.SpinnerPrinter), message string) error {
	spinner, _ := StartQodanaSpinner(message)
	if spinner == nil {
		fmt.Println(Primary(message + "..."))
	}
	fun(spinner)
	if spinner != nil {
		spinner.Success()
	}
	return nil
}

// StartQodanaSpinner starts a new spinner with the given message.
func StartQodanaSpinner(message string) (*pterm.SpinnerPrinter, error) {
	if IsInteractive() {
		QodanaSpinner.Sequence = spinnerSequence
		QodanaSpinner.MessageStyle = PrimaryStyle
		return QodanaSpinner.WithStyle(pterm.NewStyle(pterm.FgGray)).WithRemoveWhenDone(true).Start(message + "...")
	}
	return nil, nil
}

// UpdateText updates the text of the spinner.
func UpdateText(spinner *pterm.SpinnerPrinter, message string) {
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
}

// PrintProblem printProblem prints problem with source code or without it.
func PrintProblem(
	ruleId string,
	level string,
	message string,
	path string,
	line int,
	column int,
	contextLine int,
	context string,
) {
	printHeader(level, ruleId, "")
	printPath(path, line, column)
	printLines(context, contextLine, line, false)
	fmt.Print(message + "\n")
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
	fmt.Printf("%s %s\n", PrimaryBold(strings.ToUpper(level)), Primary(ruleId))
	fmt.Println(strings.Repeat(tableSep, width))
	if file != "" {
		fmt.Printf("%5s  %s %s\n", "", tableSepMid, PrimaryBold(file))
		fmt.Println(strings.Repeat(tableSep, width))
	}
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

// GetProblemsFoundMessage returns a message about the number of problems found, used in CLI and BitBucket report.
func GetProblemsFoundMessage(newProblems int) string {
	if newProblems == 0 {
		return "It seems all right 👌 No new problems found according to the checks applied"
	} else if newProblems == 1 {
		return fmt.Sprintf("Found 1 new problem according to the checks applied")
	} else {
		return fmt.Sprintf("Found %d new problems according to the checks applied", newProblems)
	}
}

// formatMessageForCI formats the message for the CI environment.
func formatMessageForCI(level, format string, a ...interface{}) string {
	message := fmt.Sprintf(format, a...)
	ci := cienvironment.DetectCIEnvironment()
	if ci != nil {
		name := qdenv.GetCIName(ci)
		if name == "github-actions" {
			return fmt.Sprintf("::%s::%s", level, message)
		} else if strings.HasPrefix(name, "azure") {
			return fmt.Sprintf("##vso[task.logissue type=%s]%s", level, message)
		} else if strings.HasPrefix(name, "circleci") {
			return fmt.Sprintf("echo '%s: %s'", level, message)
		}
	}
	return fmt.Sprintf("!  %s", message)
}
