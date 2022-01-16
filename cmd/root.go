package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tiulpin/qodana-cli/pkg"
	"io/ioutil"
	"os"
	"os/signal"
)

// NewRootCmd constructs root command.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "qodana",
		Short:   "Run Qodana CLI",
		Long:    pkg.Info,
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

// init adds all child commands to the root command.
func init() {
	RootCmd.AddCommand(
		NewInitCommand(),
		NewScanCommand(),
		NewShowCommand(),
	)
}

func Execute() error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			pkg.Interrupted = true
			log.SetOutput(ioutil.Discard)
			pkg.WarningMessage("Interrupting Qodana CLI...")
			pkg.DockerCleanup()
			_ = pkg.QodanaSpinner.Stop()
			os.Exit(0)
		}
	}()
	if err := RootCmd.Execute(); err != nil {
		return err
	}
	return nil
}
