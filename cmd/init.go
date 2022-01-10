package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana/pkg"
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
		Long:  "Configure project for Qodana: prepare Qodana configuration file by analyzing the project structure, and generate a default configuration qodana.yaml file.",
		Run: func(cmd *cobra.Command, args []string) {
			pkg.PrintProcess(
				func() { pkg.ConfigureProject(options.ProjectDir) },
				"Configuring project",
				"project configuration.")
			path, _ := filepath.Abs(options.ProjectDir)
			pkg.Primary.Printfln("Configuration is stored at %s/qodana.yaml.", path)
			pkg.Primary.Println("Run 'qodana scan' to analyze the project.")
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the project to configure")
	return cmd
}
