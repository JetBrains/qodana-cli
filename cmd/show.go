package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana/pkg"
)

type ShowOptions struct {
	ReportPath string
	Port       int
	NoBrowser  bool
}

func NewShowCommand() *cobra.Command {
	options := &ShowOptions{}
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show Qodana report",
		Long: `Show (serve locally) the latest Qodana report.

Due to JavaScript security restrictions, the generated report cannot 
be viewed via the file:// protocol (that is, by double-clicking the index.html file).
https://www.jetbrains.com/help/qodana/html-report.html
This command serves the report locally and opens browser to it.`,
		PreRun: func(cmd *cobra.Command, args []string) {},
		Run: func(cmd *cobra.Command, args []string) {
			message := fmt.Sprintf("Showing Qodana report at http://localhost:%d", options.Port)
			pkg.PrintProcess(func() { pkg.ShowReport(options.ReportPath, options.Port) }, message, "report show")
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&options.ReportPath,
		"report-path",
		".qodana/report/report",
		"Specify HTML report path (the one with index.html inside)")
	flags.IntVarP(&options.Port, "port", "p", 8080, "Specify port to serve report at")
	flags.BoolVarP(&options.NoBrowser, "no-browser", "n", false, "Do not open browser with show")
	return cmd
}
