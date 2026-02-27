package main

import (
	"github.com/JetBrains/qodana-cli/internal/cmd"
	"github.com/JetBrains/qodana-cli/internal/platform"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/spf13/cobra"
)

func Execute() {
	linter := TestLinter{}

	linterInfo := thirdpartyscan.LinterInfo{
		ProductCode:           "QDTEST",
		LinterName:            "qodana-cpp",
		LinterPresentableName: "Qodana 3rd party Test Linter",
		LinterVersion:         "0.0.1-TEST",
		IsEap:                 true,
	}

	commands := make([]*cobra.Command, 1)
	commands[0] = platform.NewThirdPartyScanCommand(linter, linterInfo)
	cmd.InitWithCustomCommands(commands)
	cmd.Execute()
}
