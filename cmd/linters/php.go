package linters

import (
	"github.com/spf13/cobra"
	"jetbrains.team/sa/cli/pkg"
)

// NewPhpCommand create new php command
func NewPhpCommand() *cobra.Command {
	options := pkg.NewLinterOptions()
	cmd := &cobra.Command{
		Use:   "php",
		Short: "Qodana PHP",
		Long:  "Qodana PHP",
		PreRun: func(cmd *cobra.Command, args []string) {
			EnsureDockerRunning()
		},
		Run: func(cmd *cobra.Command, args []string) {
			PrintHeader(options.ImageName)
			b := pkg.DefaultBuilder{}
			b.SetOptions(options)
			PrepareFolders(options)
			PrintProcess(func() { RunCommand(b.GetCommand()) })
			PrintResults(options.ReportPath)
		},
	}
	AddCommandFlags(cmd, options, "jetbrains/qodana-php")
	return cmd
}
