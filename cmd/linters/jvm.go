package linters

import (
	"github.com/spf13/cobra"
	"jetbrains.team/sa/cli/pkg"
)

// NewJvmCommand create new jvm command
func NewJvmCommand() *cobra.Command {
	options := pkg.NewLinterOptions()
	cmd := &cobra.Command{
		Use:   "jvm",
		Short: "Qodana JVM",
		Long:  "Qodana JVM",
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
	AddCommandFlags(cmd, options, "jetbrains/qodana-jvm")
	return cmd
}
