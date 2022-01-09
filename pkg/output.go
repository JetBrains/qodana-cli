package pkg

import (
	"github.com/mattn/go-isatty"
	"github.com/pterm/pterm"
	"os"
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
