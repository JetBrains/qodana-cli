package cmd

import (
	"github.com/owenrumney/go-sarif/sarif"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana/pkg"
	"os"
	"path/filepath"
)

func NewScanCommand() *cobra.Command {
	options := &pkg.LinterOptions{}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan with Qodana",
		Long:  "Scan a project with Qodana",
		PreRun: func(cmd *cobra.Command, args []string) {
			ensureDockerRunning()
		},
		Run: func(cmd *cobra.Command, args []string) {
			b := &pkg.DefaultBuilder{}
			b.SetOptions(options)
			prepareFolders(options)
			linter := readConfiguration(options)
			if err := pkg.Greet(); err != nil {
				log.Fatal("couldn't print", err)
			}
			printProcess(func() { runCommand(b.GetCommand(options, linter)) }, "Analyzing project", "project analysis")
			PrintResults(options.ReportPath)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectPath, "project-path", "p", ".", "Specify project path")
	flags.StringVar(&options.ReportPath, "report-path", ".qodana/report", "Specify report path")
	flags.StringVar(&options.CachePath, "cache-path", ".qodana/cache", "Specify cache path")
	return cmd
}

// prepareFolders cleans up report folder, creates the necessary folders for the analysis
func prepareFolders(options *pkg.LinterOptions) {
	if _, err := os.Stat(options.ReportPath); err == nil {
		err := os.RemoveAll(options.ReportPath)
		if err != nil {
			return
		}
	}
	if err := os.MkdirAll(options.CachePath, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	if err := os.MkdirAll(options.ReportPath, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
}

func readConfiguration(options *pkg.LinterOptions) string {
	qodanaYaml := pkg.GetQodanaYaml(options.ProjectPath)
	if qodanaYaml.Linters == nil {
		pkg.Error.Println(
			"No valid qodana.yaml found. Have you run `qodana init`? Running that for you...",
		)
		printProcess(func() { configureProject(options) }, "Configuring project", "project configuration")
		qodanaYaml = pkg.GetQodanaYaml(options.ProjectPath)
	}
	return qodanaYaml.Linters[0]
}

// PrintResults prints Qodana Scan result into stdout
func PrintResults(p string) {
	pcnt := 0
	s, err := sarif.Open(filepath.Join(p, "qodana.sarif.json"))
	if err != nil {
		log.Fatal(err)
	}
	for _, run := range s.Runs {
		for _, r := range run.Results {
			pcnt += 1
			ruleId := *r.RuleID
			message := *r.Message.Text
			level := *r.Level
			if len(r.Locations) > 0 {
				startLine := *r.Locations[0].PhysicalLocation.Region.StartLine
				startColumn := *r.Locations[0].PhysicalLocation.Region.StartColumn
				filePath := *r.Locations[0].PhysicalLocation.ArtifactLocation.URI
				PrintLocalizedProblem(ruleId, level, message, filePath, startLine, startColumn)
			} else {
				PrintGlobalProblem(ruleId, level, message)
			}
		}
	}

	if pcnt == 0 {
		pkg.Primary.Println("👌  It seems all right. 0 problems found according to the checks applied.\n")
	} else {
		pkg.Error.Printfln("❌  Found %d problems\n", pcnt)
	}
}

// PrintLocalizedProblem prints problem
func PrintLocalizedProblem(ruleId string, level string, message string, path string, l int, c int) {
	panels := pterm.Panels{
		{
			{Data: pkg.PrimaryBold.Sprintf("[%s]", level)},
			{Data: pkg.PrimaryBold.Sprint(ruleId)},
			{Data: pkg.Primary.Sprintf("%s:%d:%d", path, l, c)},
		},
		{
			{Data: pkg.Primary.Sprint(message)},
		},
	}
	if err := pterm.DefaultPanel.WithPanels(panels).Render(); err != nil {
		log.Fatal(err)
	}
}

// PrintGlobalProblem prints global problem
func PrintGlobalProblem(ruleId string, level string, message string) {
	panels := pterm.Panels{
		{
			{Data: pkg.PrimaryBold.Sprintf("[%s]", level)},
			{Data: pkg.PrimaryBold.Sprint(ruleId)},
		},
		{
			{Data: pkg.Primary.Sprint(message)},
		},
	}
	if err := pterm.DefaultPanel.WithPanels(panels).Render(); err != nil {
		log.Fatal(err)
	}
}
