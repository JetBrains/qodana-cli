package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const (
	productCode = "QDCLC"
	linterName  = "Qodana Community for C/C++"
)

var InterruptChannel chan os.Signal
var version = "2023.3"
var buildDateStr = "2024-04-05T10:52:23Z"

// noinspection GoUnusedFunction
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
	Execute(productCode, linterName, version, buildDateStr, true)
}
