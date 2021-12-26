package linters

import (
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana-cli/pkg"
)

// NewGoCommand is an entrypoint for go command
func NewGoCommand() *cobra.Command {
	options := pkg.NewLinterOptions()
	cmd := &cobra.Command{
		Use:   "go",
		Short: "Qodana for Go",
		Long:  "Qodana for Go",
		PreRun: func(cmd *cobra.Command, args []string) {
			EnsureDockerRunning()
		},
		Run: func(cmd *cobra.Command, args []string) {
			PrintHeader(options.ImageName)
			b := pkg.NewDefaultBuilder()
			b.SetOptions(options)
			PrepareFolders(options)
			PrintProcess(func() { RunCommand(b.GetCommand()) })
			PrintResults(options.ReportPath)
		},
	}
	AddCommandFlags(cmd, options, "jetbrains/qodana-go")
	return cmd
}
