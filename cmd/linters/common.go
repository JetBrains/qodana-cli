package linters

import (
	"fmt"
	"github.com/owenrumney/go-sarif/sarif"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana-cli/pkg"
	"os"
	"os/exec"
	"path/filepath"
)

func PrepareFolders(options *pkg.LinterOptions) {
	if err := os.MkdirAll(options.CachePath, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	if err := os.MkdirAll(options.ReportPath, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
}

func AddCommandFlags(cmd *cobra.Command, opt *pkg.LinterOptions, imageName string) {
	flags := cmd.Flags()
	flags.StringVarP(&opt.ImageName, "image", "i", imageName, "Specify qodana image")
	flags.StringVarP(&opt.ProjectPath, "project-path", "p", ".", "Specify project path")
	flags.StringVar(&opt.ReportPath, "report-path", ".qodana/report", "Specify report path")
	flags.StringVar(&opt.CachePath, "cache-path", ".qodana/cache", "Specify cache path")
}

func RunCommand(cmd *exec.Cmd) {
	log.Info("running", cmd.String())
	if err := cmd.Run(); err != nil {
		log.Fatal("failed:", err.Error())
	}
}

// PrintHeader prints the header message
func PrintHeader(image string) {
	if err := pkg.Greet(); err != nil {
		log.Fatal("couldn't print", err)
	}
	pkg.Primary.Print("🔋 Powered by ")
	pkg.PrimaryBold.Println(image)
}

// PrintProcess prints the message for processing phase
// 	TODO: Add ETA based on previous runs
func PrintProcess(f func()) {
	if err := pkg.Spin(f, "Analyzing project"); err != nil {
		log.Fatal("couldn't spin", err)
	}
	pkg.Primary.Println("✅  Analysis finished")
}

// ensureDockerInstalled checks if docker is installed
// 	TODO: Windows support?
func ensureDockerInstalled() {
	cmd := exec.Command("which", "docker")
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			pkg.Error.Println(
				"Docker is not installed on your system, ",
				"refer to https://www.docker.com/get-started for installing it",
			)
			os.Exit(1)
		}
		log.Fatal(err)
	}
}

// EnsureDockerRunning checks if docker daemon is running
func EnsureDockerRunning() {
	ensureDockerInstalled()
	cmd := exec.Command("docker", "ps")
	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			pkg.Error.Println(fmt.Sprintf(
				"Docker exited with exit code %d, perhaps docker daemon is not running?",
				exiterr.ExitCode(),
			))
			os.Exit(1)
		}
		log.Fatal(err)
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

// PrintResults prints result into stdout
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
