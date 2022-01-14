package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tiulpin/qodana-cli/pkg"
)

// NewRootCmd constructs root command
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "qodana",
		Short: "Run Qodana CLI",
		Long: `'qodana' is a command line interface for Qodana (https://jetbrains.com/qodana).
It allows you to run Qodana inspections on your local machine (or a CI agent) easily, by running Qodana Docker Images.

Documentation: https://github.com/tiulpin/qodana/blob/main/README.md

Here's a typical usage example:
- 'cd' to the project root you want to check
- run 'qodana init' in the project directory you want to check with Qodana. 
- run 'qodana scan' to scan the project.
- run 'qodana show' to explore generated Qodana report for the project.

`,
		Version: pkg.Version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logLevel, err := log.ParseLevel(viper.GetString("log-level"))
			if err != nil {
				log.Fatal(err)
			}
			log.SetLevel(logLevel)
		},
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				err := cmd.Help()
				if err != nil {
					return
				}
			}
		},
	}
	rootCmd.PersistentFlags().String("log-level", "error", "Set log-level for output")
	if err := viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		log.Fatal(err)
	}
	return rootCmd
}

var RootCmd = NewRootCmd()

// init adds all child commands to the root command
func init() {
	RootCmd.AddCommand(
		NewInitCommand(),
		NewScanCommand(),
		NewShowCommand(),
	)
}

func Execute() error {
	if err := RootCmd.Execute(); err != nil {
		return err
	}
	return nil
}
