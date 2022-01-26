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

	"github.com/mattn/go-isatty"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
)

// TODO: unify logging/error exiting messages across the codebase

// Info Two newlines at the start are important to lay the output nicely in CLI.
var Info = fmt.Sprintf(`
  %s (v%s)
  https://jetbrains.com/qodana
  Documentation – https://jb.gg/qodana-docs
  Contact us at qodana-support@jetbrains.com
  Bug Tracker: https://jb.gg/qodana-issue
  Discussions: https://jb.gg/qodana-forum
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
	SpinnerSequence  = []string{"| ", "/ ", "- ", "\\ "}
	QodanaSpinner    = pterm.DefaultSpinner
	PrimaryStyle     = pterm.NewStyle()               // PrimaryStyle is primary text style.
	PrimaryBoldStyle = pterm.NewStyle(pterm.Bold)     // PrimaryBoldStyle is primary bold text style.
	ErrorStyle       = pterm.NewStyle(pterm.FgRed)    // ErrorStyle is an error style.
	WarningStyle     = pterm.NewStyle(pterm.FgYellow) // WarningStyle is a warning style.
	MiscStyle        = pterm.NewStyle(pterm.FgGray)   // MiscStyle is a log style.
)

func Primary(text string, a ...interface{}) string {
	text = fmt.Sprintf(text, a...)
	return PrimaryStyle.Sprint(text)
}

func PrimaryBold(text string, a ...interface{}) string {
	text = fmt.Sprintf(text, a...)
	return PrimaryBoldStyle.Sprint(text)
}

// EmptyMessage is a message that is used when there is no message to show.
func EmptyMessage() {
	fmt.Println()
}

// SuccessMessage prints a success message with the icon.
func SuccessMessage(message string, a ...interface{}) {
	message = fmt.Sprintf(message, a...)
	icon := pterm.Green("✓ ")
	fmt.Println(icon, Primary(message))
}

// WarningMessage prints a warning message with the icon.
func WarningMessage(message string, a ...interface{}) {
	message = fmt.Sprintf(message, a...)
	icon := WarningStyle.Sprint("\n! ")
	fmt.Println(icon, Primary(message))
}

// ErrorMessage prints an error message with the icon.
func ErrorMessage(message string, a ...interface{}) {
	message = fmt.Sprintf(message, a...)
	icon := ErrorStyle.Sprint("✗ ")
	fmt.Println(icon, ErrorStyle.Sprint(message))
}

// printLinterLog prints the linter logs with color, when needed.
func printLinterLog(line string) {
	if strings.Contains(line, "QQQQQQ") || strings.Contains(line, "Q::") {
		PrimaryStyle.Println(line)
	} else {
		MiscStyle.Println(line)
	}
}

// PrintProcess prints the message for processing phase. TODO: Add ETA based on previous runs
func PrintProcess(f func(), start string, finished string) {
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
		Primary(message + "...")
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

// printLocalizedProblem prints problem using pterm panels.
func printLocalizedProblem(ruleId string, level string, message string, path string, l int, c int) {
	panels := pterm.Panels{
		{
			{Data: PrimaryBold("[%s]", level)},
			{Data: PrimaryBold(ruleId)},
			{Data: Primary("%s:%d:%d", path, l, c)},
		},
		{
			{Data: Primary(message)},
		},
	}
	if err := pterm.DefaultPanel.WithPanels(panels).Render(); err != nil {
		log.Fatal(err)
	}
}

// printGlobalProblem prints global problem using pterm panels.
func printGlobalProblem(ruleId string, level string, message string) {
	panels := pterm.Panels{
		{
			{Data: PrimaryBold("[%s]", level)},
			{Data: PrimaryBold(ruleId)},
		},
		{
			{Data: Primary(message)},
		},
	}
	if err := pterm.DefaultPanel.WithPanels(panels).Render(); err != nil {
		log.Fatal(err)
	}
}
