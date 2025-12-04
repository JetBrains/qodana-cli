package main

import (
	"os"

	"github.com/JetBrains/qodana-cli/internal/platform/process"
)

var InterruptChannel chan os.Signal
var version = "2023.3"
var buildDateStr = "2024-04-05T10:52:23Z"

// noinspection GoUnusedFunction
func main() {
	process.Init()
	Execute(version, buildDateStr, true)
}
