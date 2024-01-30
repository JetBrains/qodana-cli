package main

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/linter"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const (
	productCode = "QDNETC"
	linterName  = "Qodana Community for .NET"
)

var InterruptChannel chan os.Signal
var version = "2023.3"
var buildDateStr = "2023-12-05T10:52:23Z"
var isEap = true

func main() {
	InterruptChannel = make(chan os.Signal, 1)
	signal.Notify(InterruptChannel, os.Interrupt)
	signal.Notify(InterruptChannel, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-InterruptChannel
		fmt.Println("Interrupting Qodana...")
		log.SetOutput(io.Discard)
		os.Exit(0)
	}()
	linter.Execute(productCode, linterName, version, buildDateStr, isEap)
}
