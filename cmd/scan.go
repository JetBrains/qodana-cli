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
	options := pkg.NewLinterOptions()
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan with Qodana",
		Long:  "Scan a project with Qodana",
		PreRun: func(cmd *cobra.Command, args []string) {
			EnsureDockerRunning()
		},
		Run: func(cmd *cobra.Command, args []string) {
			b := pkg.NewDefaultBuilder()
			b.SetOptions(options)
			prepareFolders(options)
			linter := readConfiguration(options.ProjectPath)
			if err := pkg.Greet(); err != nil {
				log.Fatal("couldn't print", err)
			}
			PrintProcess(func() { RunCommand(b.GetCommand(options, linter)) }, "analysis")
			PrintResults(options.ReportPath)
		},
	}
	AddCommandFlags(cmd, options)
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

func readConfiguration(projectPath string) string {
	qodanaYaml := pkg.GetQodanaYaml(projectPath)
	if qodanaYaml.Linters == nil {
		pkg.Error.Println(
			"No valid qodana.yaml found. Have you run `qodana init`? ",
		)
		os.Exit(1)
	}
	return qodanaYaml.Linters[0]
}

// AddCommandFlags adds flags with the default values to the command
func AddCommandFlags(cmd *cobra.Command, opt *pkg.LinterOptions) {
	flags := cmd.Flags()
	flags.StringVarP(&opt.ProjectPath, "project-path", "p", ".", "Specify project path")
	flags.StringVar(&opt.ReportPath, "report-path", ".qodana/report", "Specify report path")
	flags.StringVar(&opt.CachePath, "cache-path", ".qodana/cache", "Specify cache path")
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
		pkg.Primary.Print("✨  Awesome code ")
		pkg.PrimaryBold.Print("0 problems ")
		pkg.Primary.Print("found!")
	} else {
		pkg.Error.Print("❌  Found ")
		pkg.ErrorBold.Printfln("%d problems", pcnt)
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
