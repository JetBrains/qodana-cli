package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/tiulpin/qodana/cmd"
	"os"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(fmt.Sprintf("error running command: %s", err))
		os.Exit(1)
	}
}
