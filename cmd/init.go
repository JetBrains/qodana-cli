package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana-cli/pkg"
	"path/filepath"
)

type ScanOptions struct {
	ProjectDir string
}

func NewInitCommand() *cobra.Command {
	options := &ScanOptions{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Configure project for Qodana",
		Long:  `Configure project for Qodana: prepare Qodana configuration file by analyzing the project structure and generating a default configuration qodana.yaml file.`,
		Run: func(cmd *cobra.Command, args []string) {
			pkg.Primary.Println()
			pkg.PrintProcess(
				func() { pkg.ConfigureProject(options.ProjectDir) },
				"Configuring project",
				"")
			path, _ := filepath.Abs(options.ProjectDir)
			pkg.Primary.Printfln(
				"Configuration is stored at %s/qodana.yaml\nRun %s to analyze the project",
				path,
				pkg.PrimaryBold.Sprint("qodana scan"),
			)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the project to configure")
	// TODO: the flag to set up supported CIs, e.g. --github tells to create .github/workflows/code_scanning.yml
	return cmd
}
