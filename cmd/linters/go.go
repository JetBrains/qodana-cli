package linters

import (
	"github.com/spf13/cobra"
	"jetbrains.team/sa/cli/pkg"
)

// NewGoCommand is an entrypoint for go command
func NewGoCommand() *cobra.Command {
	opts := pkg.NewLinterOptions()
	cmd := &cobra.Command{
		Use:   "go",
		Short: "Qodana Go",
		Long:  "Qodana Go",
		PreRun: func(cmd *cobra.Command, args []string) {
			EnsureDockerRunning()
		},
		Run: func(cmd *cobra.Command, args []string) {
			PrintHeader(opts.ImageName)
			b := pkg.NewDefaultBuilder()
			b.SetOptions(opts)
			PrepareFolders(opts)
			PrintProcess(func() { RunCommand(b.GetCommand()) })
			PrintResults(opts.ReportPath)
		},
	}
	AddCommandFlags(cmd, opts, "jetbrains/qodana-go")
	return cmd
}
