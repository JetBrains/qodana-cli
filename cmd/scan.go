package cmd

import (
	"fmt"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana/pkg"
)

func NewScanCommand() *cobra.Command {
	options := &pkg.LinterOptions{}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan project with Qodana",
		Long:  "Scan a project with Qodana",
		PreRun: func(cmd *cobra.Command, args []string) {
			pkg.EnsureDockerRunning()
			pkg.PrepareFolders(options)
		},
		Run: func(cmd *cobra.Command, args []string) {
			qodanaYaml := pkg.GetQodanaYaml(options.ProjectDir)
			ctx := cmd.Context()
			if qodanaYaml.Linters == nil {
				pkg.Warning.Println(
					"No valid qodana.yaml found. Have you run 'qodana init'? Running that for you...",
				)
				pkg.PrintProcess(func() { pkg.ConfigureProject(options) }, "Configuring project", "project configuration")
				qodanaYaml = pkg.GetQodanaYaml(options.ProjectDir)
			}
			linter := qodanaYaml.Linters[0]
			if err := pkg.Greet(); err != nil {
				log.Fatal("couldn't print", err)
			}
			docker, err := client.NewClientWithOpts()
			if err != nil {
				log.Fatal("couldn't instantiate docker client", err)
			}
			pkg.PullImage(ctx, docker, linter)
			pkg.PrintProcess(
				func() { pkg.RunLinter(cmd.Context(), docker, options, linter) },
				fmt.Sprintf("Analyzing project with %s", linter),
				"project analysis",
			)
			pkg.PrintSarif(options.ResultsDir)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVarP(&options.ResultsDir, "results-dir", "o", ".qodana/results", "Directory to save Qodana inspection results to")
	flags.StringVarP(&options.CachePath, "cache-path", "c", ".qodana/cache", "Cache directory")
	return cmd
}
