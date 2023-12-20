package linter

import (
	"github.com/JetBrains/qodana-cli/v2023/cmd"
	"github.com/JetBrains/qodana-cli/v2023/platform"
	platformcmd "github.com/JetBrains/qodana-cli/v2023/platform/cmd"
	"github.com/spf13/cobra"
)

func Execute(productCode string, linterName string, linterVersion string, buildDateStr string, isEap bool) {
	platform.CheckEAP(buildDateStr, isEap)
	options := platform.DefineOptions(func() platform.ThirdPartyOptions {
		return &CltOptions{
			LinterInfo: &platform.LinterInfo{
				ProductCode:   productCode,
				LinterName:    linterName,
				LinterVersion: linterVersion,
				IsEap:         isEap,
			},
		}
	})

	commands := make([]*cobra.Command, 1)
	commands[0] = platformcmd.NewScanCommand(options)
	cmd.InitWithCustomCommands(commands)
	cmd.Execute()
}
