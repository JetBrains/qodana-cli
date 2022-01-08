package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana/pkg"
	"path/filepath"
)

func NewInitCommand() *cobra.Command {
	options := pkg.NewLinterOptions()
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create qodana.yaml",
		Long:  "Prepare Qodana configuration file",
		PreRun: func(cmd *cobra.Command, args []string) {
			EnsureDockerRunning()
		},
		Run: func(cmd *cobra.Command, args []string) {
			PrintProcess(func() { configureProject(options) }, "configuration")
		},
	}
	AddCommandFlags(cmd, options)
	return cmd
}

func configureProject(options *pkg.LinterOptions) {
	path, _ := filepath.Abs(options.ProjectPath)
	linters := getProjectLinters(options)
	if len(linters) == 0 {
		log.Fatal("Qodana does not support the project " + path + " yet. See https://www.jetbrains.com/help/qodana/supported-technologies.html")
	}
	pkg.WriteQodanaYaml(options.ProjectPath, linters)
}

func getProjectLinters(options *pkg.LinterOptions) []string {
	langLinters := map[string]string{
		"Java":       "jetbrains/qodana-jvm",
		"Kotlin":     "jetbrains/qodana-jvm",
		"Python":     "jetbrains/qodana-python",
		"PHP":        "jetbrains/qodana-php",
		"JavaScript": "jetbrains/qodana-js",
		"TypeScript": "jetbrains/qodana-js",
	}
	var linters []string
	languages, _ := pkg.RecognizeDirLanguages(options.ProjectPath)
	for language, _ := range languages {
		if linter, err := langLinters[language]; err {
			linters = append(linters, linter)
		}
	}
	return linters
}
