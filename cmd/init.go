package cmd

import (
	"path/filepath"

	"github.com/JetBrains/qodana-cli/core"
	"github.com/spf13/cobra"
)

// InitOptions represents scan command options.
type InitOptions struct {
	ProjectDir string
}

// NewInitCommand returns a new instance of the show command.
func NewInitCommand() *cobra.Command {
	options := &InitOptions{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Configure project for Qodana",
		Long:  `Configure project for Qodana: prepare Qodana configuration file by analyzing the project structure and generating a default configuration qodana.yaml file.`,
		Run: func(cmd *cobra.Command, args []string) {
			core.Primary.Println() // TODO
			core.PrintProcess(
				func() { core.ConfigureProject(options.ProjectDir) },
				"Configuring project",
				"")
			path, _ := filepath.Abs(options.ProjectDir)
			core.Primary.Printfln(
				"Configuration is stored at %s/qodana.yaml\nRun %s to analyze the project",
				path,
				core.PrimaryBold.Sprint("qodana scan"),
			)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the project to configure")
	// TODO: the flag to set up supported CIs, e.g. --github tells to create .github/workflows/code_scanning.yml
	return cmd
}
