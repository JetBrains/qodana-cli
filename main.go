package main

import (
	"fmt"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"github.com/tiulpin/qodana/cmd"
	"github.com/tiulpin/qodana/pkg"
	"os"
)

func main() {
	if !pkg.IsInteractive() {
		pterm.DisableColor()
	}
	if err := cmd.Execute(); err != nil {
		log.Fatal(fmt.Sprintf("error running command: %s", err))
		os.Exit(1)
	}
}
