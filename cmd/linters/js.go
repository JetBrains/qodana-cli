package linters

import (
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana-cli/pkg"
)

func NewJsCommand() *cobra.Command {
	options := pkg.NewLinterOptions()
	cmd := &cobra.Command{
		Use:   "js",
		Short: "Qodana for JS",
		Long:  "Qodana for JS",
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
	AddCommandFlags(cmd, options, "jetbrains/qodana-js")
	return cmd
}

// JsCmd is an entry point for running javascript qodana linter
