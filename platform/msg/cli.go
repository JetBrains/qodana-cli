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
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
)

var qodanaInteractiveConfirm = pterm.InteractiveConfirmPrinter{
	DefaultValue: true,
	DefaultText:  DefaultPromptText,
	TextStyle:    PrimaryStyle,
	ConfirmText:  "Yes",
	ConfirmStyle: PrimaryStyle,
	RejectText:   "No",
	RejectStyle:  PrimaryStyle,
	SuffixStyle:  PrimaryStyle,
}

// AskUserConfirm asks the user for confirmation with yes/no.
func AskUserConfirm(what string) bool {
	if !IsInteractive() {
		return false
	}
	prompt := qodanaInteractiveConfirm
	prompt.DefaultText = "\n?  " + what
	answer, err := prompt.Show()
	if err != nil {
		log.Fatalf("Error while waiting for user input: %s", err)
	}
	return answer
}
