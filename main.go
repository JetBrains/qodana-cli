package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"jetbrains.team/sa/cli/cmd"
	"os"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(fmt.Sprintf("error running command: %s", err))
		os.Exit(1)
	}
}
