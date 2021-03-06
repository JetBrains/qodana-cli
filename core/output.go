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

package core

import (
	"fmt"
	"os"
	"strings"

	"github.com/liamg/clinch/terminal"
	"github.com/mattn/go-isatty"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
)

// Info Two newlines at the start are important to lay the output nicely in CLI.
var Info = fmt.Sprintf(`
  %s (v%s)
  https://jb.gg/qodana-cli
  Documentation – https://jb.gg/qodana-docs
  Contact us at qodana-support@jetbrains.com
  Bug Tracker: https://jb.gg/qodana-issue
  Community forum: https://jb.gg/qodana-forum
`, "Qodana CLI", Version)

// IsInteractive returns true if the current execution environment is interactive (useful for colors/animations toggle).
func IsInteractive() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) && os.Getenv("NO_INTERACTIVE") == ""
}

// DisableColor disables colors in the output.
func DisableColor() {
	pterm.DisableColor()
}

// styles and different declarations intended to be used only inside this file
var (
	NoLineWidth             = 7
	SpinnerSequence         = []string{"| ", "/ ", "- ", "\\ "}
	QodanaSpinner           = pterm.DefaultSpinner
	PrimaryStyle            = pterm.NewStyle()               // PrimaryStyle is a primary text style.
	PrimaryBoldStyle        = pterm.NewStyle(pterm.Bold)     // PrimaryBoldStyle is a primary bold text style.
	ErrorStyle              = pterm.NewStyle(pterm.FgRed)    // ErrorStyle is an error style.
	WarningStyle            = pterm.NewStyle(pterm.FgYellow) // WarningStyle is a warning style.
	MiscStyle               = pterm.NewStyle(pterm.FgGray)   // MiscStyle is a log style.
	TableSep                = MiscStyle.Sprint("─")
	TableSepUp              = MiscStyle.Sprint("┬")
	TableSepMid             = MiscStyle.Sprint("│")
	TableSepDown            = MiscStyle.Sprint("┴")
	TableUp                 = strings.Repeat(TableSep, NoLineWidth) + TableSepUp
	TableDown               = strings.Repeat(TableSep, NoLineWidth) + TableSepDown
	QodanaInteractiveSelect = pterm.InteractiveSelectPrinter{
		TextStyle:     PrimaryStyle,
		DefaultText:   "Please select the linter",
		Options:       []string{},
		OptionStyle:   PrimaryStyle,
		DefaultOption: "",
		MaxHeight:     5,
		Selector:      ">",
		SelectorStyle: PrimaryStyle,
	}
	QodanaInteractiveConfirm = pterm.InteractiveConfirmPrinter{
		DefaultValue: true,
		DefaultText:  "Do you want to open the results (logs) directory?",
		TextStyle:    PrimaryStyle,
		ConfirmText:  "Yes",
		ConfirmStyle: PrimaryStyle,
		RejectText:   "No",
		RejectStyle:  PrimaryStyle,
		SuffixStyle:  PrimaryStyle,
	}
)

// Primary prints a message in the primary style.
func Primary(text string, a ...interface{}) string {
	text = fmt.Sprintf(text, a...)
	return PrimaryStyle.Sprint(text)
}

// PrimaryBold prints a message in the primary bold style.
func PrimaryBold(text string, a ...interface{}) string {
	text = fmt.Sprintf(text, a...)
	return PrimaryBoldStyle.Sprint(text)
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
	icon := WarningStyle.Sprint("\n! ")
	pterm.Println(icon, Primary(message))
}

// ErrorMessage prints an error message with the icon.
func ErrorMessage(message string, a ...interface{}) {
	message = fmt.Sprintf(message, a...)
	icon := ErrorStyle.Sprint("✗ ")
	pterm.Println(icon, ErrorStyle.Sprint(message))
}

// printLinterLog prints the linter logs with color, when needed.
func printLinterLog(line string) {
	if strings.Contains(line, " / /") ||
		strings.Contains(line, "_              _") ||
		strings.Contains(line, "\\/__") ||
		strings.Contains(line, "\\ \\") {
		PrimaryStyle.Println(line)
	} else {
		MiscStyle.Println(line)
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
		fmt.Println(Primary(message + "..."))
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
		QodanaSpinner.Sequence = SpinnerSequence
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

// printProblem prints problem with source code or without it.
func printProblem(
	ruleId string,
	level string,
	message string,
	path string,
	line int,
	column int,
	contextLine int,
	context string,
) {
	width, _ := terminal.Size()
	if width <= 0 {
		width = 80
	}
	fmt.Printf("\n%s %s\n", PrimaryBold(strings.ToUpper(level)), Primary(ruleId))
	fmt.Println(strings.Repeat(TableSep, width))
	if path != "" && line > 0 && column > 0 {
		fmt.Printf(" %s:%d:%d\n", path, line, column)
		fmt.Printf("%s%s\n", TableUp, strings.Repeat(TableSep, width-NoLineWidth-1))
	} else {
		fmt.Println(strings.Repeat(TableSep, width))
	}
	if contextLine > 0 && context != "" {
		code := strings.Split(context, "\n")
		for i := 0; i < len(code); i++ {
			var printLine string
			currentLine := contextLine + i
			if currentLine == line {
				printLine = ErrorStyle.Sprint(code[i]) + " ←"
			} else {
				printLine = WarningStyle.Sprint(code[i])
			}
			lineNumber := MiscStyle.Sprintf("%5d", currentLine)
			fmt.Printf("%s  %s %s\n", lineNumber, TableSepMid, printLine)
		}
		fmt.Printf("%s%s\n", TableDown, strings.Repeat(TableSep, width-NoLineWidth-1))
	}
	fmt.Printf("%s\n\n", message)
}
