package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana/pkg"
	"os"
	"path/filepath"
)

func NewInitCommand() *cobra.Command {
	options := &pkg.LinterOptions{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create qodana.yaml",
		Long:  "Prepare Qodana configuration file",
		PreRun: func(cmd *cobra.Command, args []string) {
			ensureDockerRunning()
		},
		Run: func(cmd *cobra.Command, args []string) {
			printProcess(func() { configureProject(options) }, "Configuring project", "project configuration. Check qodana.yaml.")
			pkg.Primary.Println("🚀  Run `qodana scan` to analyze the project\n")
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectPath, "project-path", "p", ".", "Specify project path")
	return cmd
}

func configureProject(options *pkg.LinterOptions) {
	path, _ := filepath.Abs(options.ProjectPath)
	linters := getProjectLinters(options)
	if len(linters) == 0 {
		pkg.Error.Printfln(
			"Qodana does not support the project %s yet. See https://www.jetbrains.com/help/qodana/supported-technologies.html",
			path,
		)
		os.Exit(1)
	}
	pkg.WriteQodanaYaml(options.ProjectPath, linters)
}

func getProjectLinters(options *pkg.LinterOptions) []string {
	var linters []string
	langLinters := map[string]string{
		"Java":       "jetbrains/qodana-jvm",
		"Kotlin":     "jetbrains/qodana-jvm",
		"Python":     "jetbrains/qodana-python",
		"PHP":        "jetbrains/qodana-php",
		"JavaScript": "jetbrains/qodana-js",
		"TypeScript": "jetbrains/qodana-js",
	}
	languages := pkg.ReadIdeaFolder(options.ProjectPath)
	if len(languages) == 0 {
		languages, _ = pkg.RecognizeDirLanguages(options.ProjectPath)
	}
	for _, language := range languages {
		if linter, err := langLinters[language]; err {
			if !pkg.Contains(linters, linter) {
				linters = append(linters, linter)
			}
		}
	}
	return linters
}
