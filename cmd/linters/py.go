package linters

import (
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana-cli/pkg"
)

// NewPyCommand create new py command
func NewPyCommand() *cobra.Command {
	options := pkg.NewLinterOptions()
	cmd := &cobra.Command{
		Use:   "py",
		Short: "Qodana Python",
		Long:  "Qodana Python",
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
	AddCommandFlags(cmd, options, "jetbrains/qodana-python")
	return cmd
}
