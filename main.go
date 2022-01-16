package main

import (
	"fmt"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"github.com/tiulpin/qodana-cli/cmd"
	"github.com/tiulpin/qodana-cli/pkg"
	"os"
)

func main() {
	if !pkg.IsInteractive() || os.Getenv("NO_COLOR") != "" { // http://no-color.org
		pterm.DisableColor()
	}
	if os.Getenv("DO_NOT_TRACK") == "1" { // https://consoledonottrack.com
		pkg.DoNotTrack = true
	}
	if err := cmd.Execute(); err != nil {
		log.Fatal(fmt.Sprintf("error running command: %s", err))
		os.Exit(1)
	}
}
