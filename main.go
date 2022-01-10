package main

import (
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"github.com/tiulpin/qodana/cmd"
	"github.com/tiulpin/qodana/pkg"
	"os"
	"runtime"
	"time"
)

var sentryDsn string

func main() {
	if !pkg.IsInteractive() || os.Getenv("NO_COLOR") != "" {
		pterm.DisableColor()
	}
	if os.Getenv("DO_NOT_TRACK") == "1" {
		pkg.DoNotTrack = true
	}
	if !pkg.DoNotTrack && sentryDsn != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              sentryDsn,
			TracesSampleRate: 0.5,
			Environment:      runtime.GOOS,
			Release:          "qodana-cli@0.3.0",
			Debug:            false,
		})
		if err != nil {
			log.Fatalf("sentry.Init: %s", err)
		}
	}
	defer sentry.Flush(2 * time.Second)

	if err := cmd.Execute(); err != nil {
		log.Fatal(fmt.Sprintf("error running command: %s", err))
		os.Exit(1)
	}
}
