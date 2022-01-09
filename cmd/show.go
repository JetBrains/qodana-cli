package cmd

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type ShowOptions struct {
	ReportPath string
	Port       int
	NoBrowser  bool
}

func NewShowCommand() *cobra.Command {
	options := &ShowOptions{}
	cmd := &cobra.Command{
		Use:    "show",
		Short:  "Show Qodana report",
		Long:   "Show (serve locally) the latest Qodana report",
		PreRun: func(cmd *cobra.Command, args []string) {},
		Run: func(cmd *cobra.Command, args []string) {
			checkReport(options.ReportPath)
			message := fmt.Sprintf("Showing Qodana report at http://localhost:%d", options.Port)
			printProcess(func() { showReport(options) }, message, "report show")
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&options.ReportPath, "report-path", ".qodana/report/report", "Specify HTML report path (the one with index.html inside)")
	flags.IntVar(&options.Port, "port", 8080, "Specify port to serve report at")
	flags.BoolVar(&options.NoBrowser, "no-browser", false, "Do not open browser with show")
	return cmd
}

func checkReport(reportPath string) {
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		log.Fatal("Qodana report at  not found. Get the report by running `qodana scan`")
	}
}

// showReport serves the Qodana report
func showReport(options *ShowOptions) {
	url := fmt.Sprintf("http://localhost:%d", options.Port)
	go func() {
		err := openBrowser(url)
		if err != nil {
			log.Fatal(err.Error())
		}
	}()
	http.Handle("/", http.FileServer(http.Dir(filepath.Join(options.ReportPath, "report"))))
	err := http.ListenAndServe(fmt.Sprintf(":%d", options.Port), nil)
	if err != nil {
		return
	}
}

// openBrowser opens the default browser to the given url
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}
