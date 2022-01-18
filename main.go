package main

import (
	"fmt"
	"os"

	"github.com/JetBrains/qodana-cli/cmd"
	"github.com/JetBrains/qodana-cli/core"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
)

func main() {
	if !core.IsInteractive() || os.Getenv("NO_COLOR") != "" { // http://no-color.org
		pterm.DisableColor()
	}
	if os.Getenv("DO_NOT_TRACK") == "1" { // https://consoledonottrack.com
		core.DoNotTrack = true
	}
	if err := cmd.Execute(); err != nil {
		log.Fatal(fmt.Sprintf("error running command: %s", err))
		os.Exit(1)
	}
}
