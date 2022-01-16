package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana-cli/pkg"
	"path/filepath"
)

type ShowOptions struct {
	ReportDir string
	Port      int
}

func NewShowCommand() *cobra.Command {
	options := &ShowOptions{}
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show Qodana report",
		Long: `Show (serve locally) the latest Qodana report.

Due to JavaScript security restrictions, the generated report cannot
be viewed via the file:// protocol (by double-clicking the index.html file).
https://www.jetbrains.com/help/qodana/html-report.html
This command serves the Qodana report locally and opens a browser to it.`,
		Run: func(cmd *cobra.Command, args []string) {
			if options.ReportDir == "" {
				linter := pkg.GetQodanaYaml(".").Linter
				if linter == "" {
					log.Fatalf("Can't automatically find the report...\n" +
						"Please specify the report directory with the --report-dir flag.")
				}
				options.ReportDir = filepath.Join(pkg.GetLinterHome(".", linter), "results", "report")
			}
			pkg.ShowReport(options.ReportDir, options.Port)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ReportDir,
		"report-dir",
		"r",
		"",
		"Specify HTML report path (the one with index.html inside) (default .qodana/<linter>/results/report)")
	flags.IntVarP(&options.Port, "port", "p", 8080, "Specify port to serve report at")
	return cmd
}
