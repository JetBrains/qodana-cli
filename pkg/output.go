package pkg

import (
	"github.com/mattn/go-isatty"
	"github.com/owenrumney/go-sarif/sarif"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

const logo = `
         =+++++=-     +++++++=-      
      -+@@@+++@@@@+  -@@@@+@@@@@+-   
     =@@@=      =@@@ -@@@     =@@@=  
     @@@+        +@@+ @@@      =@@@  
     +@@@    +@+=@@@--@@@      +@@+  
      +@@@=- =@@@@@+ -@@@----=@@@+   
       -+@@@@@@@++@@--@@@@@@@@@=-    
            --    -                  
`

// Two newlines at the start are important to lay the output nicely in CLI
const info = `

Qodana CLI
Documentation – https://jb.gg/qodana-docs
Contact us at qodana-support@jetbrains.com
Or via our issue tracker – https://jb.gg/qodana-issue
Or share your feedback in our Slack – https://jb.gg/qodana-slack
`

// IsInteractive returns true if the current execution environment is interactive (useful for colors/animations toggle)
func IsInteractive() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
}

// Greet prints welcome message
func Greet() error {
	panels := pterm.Panels{{
		{Data: PrimaryBold.Sprint(logo)},
		{Data: Accent.Sprint(info)},
	}}
	return pterm.DefaultPanel.WithPanels(panels).Render()
}

// PrintProcess prints the message for processing phase. TODO: Add ETA based on previous runs
func PrintProcess(f func(), start string, finished string) {
	if err := Spin(f, start); err != nil {
		log.Fatal("\nProblem occurred:", err.Error())
	}
	Primary.Printfln("✅  Finished %s ", finished)
}

// Spin creates spinner and runs the given function. Also, spin is a spider in Dutch
func Spin(fun func(), message string) error {
	if IsInteractive() {
		spinner, err := pterm.DefaultSpinner.Start(message + "...")
		if err != nil {
			return err
		}
		spinner.RemoveWhenDone = true
		spinner.Style = pterm.NewStyle(pterm.FgLightMagenta)
		fun()
		spinner.Success()
	} else {
		pterm.DefaultBasicText.Println(message + "...")
		fun()
	}
	return nil
}

// Primary is primary text style
var Primary = pterm.NewStyle()

// PrimaryBold is primary bold text style
var PrimaryBold = pterm.NewStyle(pterm.Bold)

// Accent is an accent style
var Accent = pterm.NewStyle(pterm.FgMagenta)

// Error is an error style
var Error = pterm.NewStyle(pterm.FgRed)

// ErrorBold is a bold error style
var ErrorBold = pterm.NewStyle(pterm.FgRed, pterm.Bold)

// PrintSarif prints Qodana Scan result into stdout
func PrintSarif(p string) {
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
		Primary.Println("👌  It seems all right. 0 problems found according to the checks applied.")
	} else {
		Error.Printfln("❌  Found %d problems", pcnt)
	}
}

// PrintLocalizedProblem prints problem
func PrintLocalizedProblem(ruleId string, level string, message string, path string, l int, c int) {
	panels := pterm.Panels{
		{
			{Data: PrimaryBold.Sprintf("[%s]", level)},
			{Data: PrimaryBold.Sprint(ruleId)},
			{Data: Primary.Sprintf("%s:%d:%d", path, l, c)},
		},
		{
			{Data: Primary.Sprint(message)},
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
			{Data: PrimaryBold.Sprintf("[%s]", level)},
			{Data: PrimaryBold.Sprint(ruleId)},
		},
		{
			{Data: Primary.Sprint(message)},
		},
	}
	if err := pterm.DefaultPanel.WithPanels(panels).Render(); err != nil {
		log.Fatal(err)
	}
}
