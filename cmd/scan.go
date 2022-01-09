package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana/pkg"
)

func NewScanCommand() *cobra.Command {
	options := &pkg.LinterOptions{}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan with Qodana",
		Long:  "Scan a project with Qodana",
		PreRun: func(cmd *cobra.Command, args []string) {
			pkg.EnsureDockerRunning()
		},
		Run: func(cmd *cobra.Command, args []string) {
			b := &pkg.DefaultBuilder{}
			b.SetOptions(options)
			qodanaYaml := pkg.GetQodanaYaml(options.ProjectPath)
			if qodanaYaml.Linters == nil {
				pkg.Error.Println(
					"No valid qodana.yaml found. Have you run `qodana init`? Running that for you...",
				)
				pkg.PrintProcess(func() { pkg.ConfigureProject(options) }, "Configuring project", "project configuration")
				qodanaYaml = pkg.GetQodanaYaml(options.ProjectPath)
			}
			linter := qodanaYaml.Linters[0]
			if err := pkg.Greet(); err != nil {
				log.Fatal("couldn't print", err)
			}
			pkg.PrintProcess(func() { pkg.RunCommand(b.GetDockerCommand(options, linter)) }, "Analyzing project", "project analysis")
			pkg.PrintSarif(options.ReportPath)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectPath, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVarP(&options.ReportPath, "results-dir", "o", ".qodana/results", "Directory to save Qodana inspection results to")
	flags.StringVarP(&options.CachePath, "cache-path", "c", ".qodana/cache", "Cache directory")
	return cmd
}
