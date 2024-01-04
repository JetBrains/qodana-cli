package platform

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
