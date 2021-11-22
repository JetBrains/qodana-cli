package cmd

import (
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"jetbrains.team/sa/cli/cmd/linters"
	"jetbrains.team/sa/cli/pkg"
)

var cfgFile string

// RootCmd TODO: Update wording here, please :)
var RootCmd = &cobra.Command{
	Use:   "qodana",
	Short: "Qodana is a fantastic code quality tool",
	Long:  "Abracadabra, abracadabra, abracadabra",
	Run: func(cmd *cobra.Command, args []string) {
		linter, err := pkg.Predict(".")
		if err != nil {
			log.Fatal("can't predict: ", err)
		}
		switch linter {
		case pkg.JVM:
			err = linters.JvmCmd.Execute()
		case pkg.Go:
			err = linters.GoCmd.Execute()
		case pkg.PHP:
			err = linters.PhpCmd.Execute()
		case pkg.JavaScript:
			err = linters.JsCmd.Execute()
		case pkg.Python:
			err = linters.PyCmd.Execute()
		default:
			err = errors.New(fmt.Sprintf("no linter exists for %s", linter))
		}
		if err != nil {
			log.Fatal("failed running command: ", err)
		}

		log.Print("predicted linter: ", linter)
	},
}

// initConfig TODO: Use viper probably, read qodana-yaml
func initConfig() {
}

func init() {
	cobra.OnInitialize(initConfig)
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "qodana.yaml", "config file (default is ./qodana.yaml)")
	RootCmd.AddCommand(
		linters.JsCmd,
		linters.JvmCmd,
		linters.PhpCmd,
		linters.PyCmd,
		linters.GoCmd,
	)
}

func Execute() error {
	if err := RootCmd.Execute(); err != nil {
		return err
	}
	return nil
}
