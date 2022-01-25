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

//goland:noinspection GoUnusedGlobalVariable
var (
	SpinnerSequence = []string{"| ", "/ ", "- ", "\\ "}
	QodanaSpinner   = pterm.DefaultSpinner
	Primary         = pterm.NewStyle()                // Primary is primary text style.
	PrimaryBold     = pterm.NewStyle(pterm.Bold)      // PrimaryBold is primary bold text style.
	Accent          = pterm.NewStyle(pterm.FgMagenta) // Accent is an accent style.
	Error           = pterm.NewStyle(pterm.FgRed)     // Error is an error style.
	Warning         = pterm.NewStyle(pterm.FgYellow)  // Warning is a warning style.
	Misc            = pterm.NewStyle(pterm.FgGray)    // Misc is a log style.
)

// IsInteractive returns true if the current execution environment is interactive (useful for colors/animations toggle).
func IsInteractive() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) && os.Getenv("NO_INTERACTIVE") == ""
}

// SuccessMessage print success message with the icon.
func SuccessMessage(message string) {
	icon := pterm.Green("✓ ")
	pterm.Println(icon, Primary.Sprint(message))
}

// WarningMessage print warning message with the icon.
func WarningMessage(message string) {
	icon := Warning.Sprint("\n! ")
	pterm.Println(icon, Primary.Sprint(message))
}

// ErrorMessage print error message with the icon.
func ErrorMessage(message string) {
	icon := pterm.Red("✗ ")
	pterm.Println(icon, Error.Sprint(message))
}

// printLinterLog prints linter logs with color, when needed.
func printLinterLog(line string) {
	if strings.Contains(line, "QQQQQQ") || strings.Contains(line, "Q::") {
		Primary.Println(line)
	} else {
		Misc.Println(line)
	}
}

// PrintProcess prints the message for processing phase. TODO: Add ETA based on previous runs
func PrintProcess(f func(), start string, finished string) {
	if err := spin(f, start); err != nil {
		log.Fatal("\nProblem occurred:", err.Error())
	}
	if finished != "" {
		SuccessMessage(fmt.Sprintf("Finished %s", finished))
	}
}

// spin creates spinner and runs the given function. Also, spin is a spider in Dutch.
func spin(fun func(), message string) error {
	if IsInteractive() {
		spinner, _ := StartQodanaSpinner(message)
		fun()
		spinner.Success()
	} else {
		pterm.DefaultBasicText.Println(message + "...")
		fun()
		pterm.DefaultBasicText.Println(message + "...")
	}
	return nil
}

// StartQodanaSpinner starts a new spinner with the given message.
func StartQodanaSpinner(message string) (*pterm.SpinnerPrinter, error) {
	QodanaSpinner.Sequence = SpinnerSequence
	return QodanaSpinner.WithStyle(pterm.NewStyle(pterm.FgGray)).WithRemoveWhenDone(true).Start(message + "...")
}

// updateText updates the text of the spinner.
func updateText(spinner *pterm.SpinnerPrinter, message string) {
	if spinner != nil {
		spinner.UpdateText(message + "...")
	}
}

// PrintLocalizedProblem prints problem using pterm panels.
func PrintLocalizedProblem(ruleId string, level string, message string, path string, l int, c int) {
	panels := pterm.Panels{
		{
			{Data: PrimaryBold.Sprintf("[%s]", level)},
			{Data: PrimaryBold.Sprint(ruleId)},
			{Data: Primary.Sprintf("%s:%d:%d", path, l, c)},
		},
		{
			{Data: Primary.Sprint(message)},
		},
	}
	if err := pterm.DefaultPanel.WithPanels(panels).Render(); err != nil {
		log.Fatal(err)
	}
}

// PrintGlobalProblem prints global problem using pterm panels.
func PrintGlobalProblem(ruleId string, level string, message string) {
	panels := pterm.Panels{
		{
			{Data: PrimaryBold.Sprintf("[%s]", level)},
			{Data: PrimaryBold.Sprint(ruleId)},
		},
		{
			{Data: Primary.Sprint(message)},
		},
	}
	if err := pterm.DefaultPanel.WithPanels(panels).Render(); err != nil {
		log.Fatal(err)
	}
}
