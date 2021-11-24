package linters

import (
	"github.com/spf13/cobra"
	"jetbrains.team/sa/cli/pkg"
)

func NewJsCommand() *cobra.Command {
	opts := pkg.NewLinterOptions()
	cmd := &cobra.Command{
		Use:   "js",
		Short: "Qodana JavaScript",
		Long:  "Qodana JavaScript",
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
	AddCommandFlags(cmd, opts, "jetbrains/qodana-js")
	return cmd
}

// JsCmd is an entry point for running javascript qodana linter
