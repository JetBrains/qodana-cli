package main

import (
	"github.com/JetBrains/qodana-cli/v2025/cmd"
	"github.com/JetBrains/qodana-cli/v2025/platform"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"
	"github.com/spf13/cobra"
)

func Execute(
	linterVersion string,
	buildDateStr string,
	isEap bool,
) {
	platform.CheckEAP(buildDateStr, isEap)

	linter := ClangLinter{}

	linterInfo := thirdpartyscan.LinterInfo{
		ProductCode:           product.ClangLinter.ProductCode,
		LinterName:            product.ClangLinter.Name,
		LinterPresentableName: product.ClangLinter.PresentableName,
		LinterVersion:         linterVersion,
		IsEap:                 isEap,
	}

	commands := make([]*cobra.Command, 1)
	commands[0] = platform.NewThirdPartyScanCommand(linter, linterInfo)
	cmd.InitWithCustomCommands(commands)
	cmd.Execute()
}
