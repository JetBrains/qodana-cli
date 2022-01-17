package pkg

import (
	"fmt"
	"github.com/mattn/go-isatty"
	"github.com/owenrumney/go-sarif/sarif"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
)

// TODO: unify logging/error exiting messages across the codebase

// https://patorjk.com/software/taag/#p=testall&f=Impossible&t=QD
const logo = `
          _            _         
         /\ \         /\ \       
        /  \ \       /  \ \____  
       / /\ \ \     / /\ \_____\ 
      / / /\ \ \   / / /\/___  / 
     / / /  \ \_\ / / /   / / /  
    / / / _ / / // / /   / / /   
   / / / /\ \/ // / /   / / /    
  / / /__\ \ \/ \ \ \__/ / /     
 / / /____\ \ \  \ \___\/ /      
 \/________\_\/   \/_____/`

// Info Two newlines at the start are important to lay the output nicely in CLI.
var Info = Accent.Sprintf(`
  %s (v%s)
  https://jetbrains.com/qodana
  Documentation – https://jb.gg/qodana-docs
  Contact us at qodana-support@jetbrains.com
  Bug Tracker: https://jb.gg/qodana-issue
  Discussions: https://jb.gg/qodana-forum
`, PrimaryBold.Sprint("Qodana CLI"), Version)

func licenseWarning(message string, image string) string {
	linters := []string{
		fmt.Sprintf("By using %s Docker image, you agree to", PrimaryBold.Sprint(image)),
		"   - JetBrains Privacy Policy (https://jb.gg/jetbrains-privacy-policy)",
	}
	var agreement string
	if strings.Contains(message, "Qodana Community Linters Agreement") {
		agreement = "   - Qodana Community Linters Agreement (https://jb.gg/qodana-community-linters)"
	} else {
		agreement = "   - JETBRAINS EAP USER AGREEMENT (https://jb.gg/jetbrains-user-eap)"
	}
	return strings.Join(append(linters, agreement, ""), "\n")
}

// IsInteractive returns true if the current execution environment is interactive (useful for colors/animations toggle).
func IsInteractive() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) && os.Getenv("NO_INTERACTIVE") == ""
}

// Primary is primary text style.
var Primary = pterm.NewStyle()

// PrimaryBold is primary bold text style.
var PrimaryBold = pterm.NewStyle(pterm.Bold)

// Accent is an accent style
var Accent = pterm.NewStyle(pterm.FgMagenta)

// Error is an error style.
var Error = pterm.NewStyle(pterm.FgRed)

// ErrorBold is a bold error style.
//goland:noinspection GoUnusedGlobalVariable
var ErrorBold = pterm.NewStyle(pterm.FgRed, pterm.Bold)

// Warning is a warning style
var Warning = pterm.NewStyle(pterm.FgYellow)

//goland:noinspection GoUnusedGlobalVariable
var WarningBold = pterm.NewStyle(pterm.FgYellow, pterm.Bold)

func SuccessMessage(message string) {
	icon := pterm.Green("✓ ")
	pterm.Println(icon, Primary.Sprint(message))
}

func WarningMessage(message string) {
	icon := Warning.Sprint("\n! ")
	pterm.Println(icon, Primary.Sprint(message))
}

func ErrorMessage(message string) {
	icon := pterm.Red("✗ ")
	pterm.Println(icon, Error.Sprint(message))
}

var SpinnerSequence = []string{"| ", "/ ", "- ", "\\ "}

var QodanaSpinner = pterm.DefaultSpinner

// Greet prints welcome message
func Greet() error {
	panels := pterm.Panels{{
		{Data: PrimaryBold.Sprint(logo)},
		{Data: "\n\n\n" + Accent.Sprint(Info)},
	}}
	return pterm.DefaultPanel.WithPanels(panels).Render()
}

// PrintProcess prints the message for processing phase. TODO: Add ETA based on previous runs
func PrintProcess(f func(), start string, finished string) {
	if err := spin(f, start); err != nil {
		log.Fatal("\nProblem occurred:", err.Error())
	}
	if finished != "" {
		SuccessMessage(fmt.Sprintf("Finished %s", finished))
	}
}

// spin creates spinner and runs the given function. Also, spin is a spider in Dutch.
func spin(fun func(), message string) error {
	if IsInteractive() {
		spinner, _ := StartQodanaSpinner(message)
		fun()
		spinner.Success()
	} else {
		pterm.DefaultBasicText.Println(message + "...")
		fun()
		pterm.DefaultBasicText.Println(message + "...")
	}
	return nil
}

func StartQodanaSpinner(message string) (*pterm.SpinnerPrinter, error) {
	QodanaSpinner.Sequence = SpinnerSequence
	return QodanaSpinner.WithStyle(pterm.NewStyle(pterm.FgGray)).WithRemoveWhenDone(true).Start(message + "...")
}

func updateText(spinner *pterm.SpinnerPrinter, message string) {
	if spinner != nil {
		spinner.UpdateText(message + "...")
	} else {
		pterm.DefaultBasicText.Print(message + "..." + "\n")
	}
}

// PrintSarif prints Qodana Scan result into stdout
func PrintSarif(p string, b bool) { // TODO: read the number of problems directly from SARIF and prepare a summary table
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
				if b {
					PrintLocalizedProblem(ruleId, level, message, filePath, startLine, startColumn)
				}
			} else {
				if b {
					PrintGlobalProblem(ruleId, level, message)
				}
			}
		}
	}

	if pcnt == 0 {
		SuccessMessage("It seems all right 👌 No problems found according to the checks applied")
	} else {
		ErrorMessage(fmt.Sprintf("Qodana found %d problems according to the checks applied", pcnt))
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
