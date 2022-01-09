package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana/pkg"
)

func NewInitCommand() *cobra.Command {
	options := &pkg.LinterOptions{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create qodana.yaml",
		Long:  "Prepare Qodana configuration file",
		PreRun: func(cmd *cobra.Command, args []string) {
			pkg.EnsureDockerRunning()
		},
		Run: func(cmd *cobra.Command, args []string) {
			pkg.PrintProcess(
				func() { pkg.ConfigureProject(options) },
				"Configuring project",
				"project configuration. Check qodana.yaml.")
			pkg.Primary.Println("🚀  Run `qodana scan` to analyze the project")
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVarP(&options.ResultsDir, "results-dir", "o", ".qodana/results", "Directory to save Qodana inspection results to")
	flags.StringVarP(&options.CachePath, "cache-path", "c", ".qodana/cache", "Cache directory")
	return cmd
}
