package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tiulpin/qodana-cli/cmd/linters"
	"github.com/tiulpin/qodana-cli/pkg"
)

// NewRootCmd constructs root command
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "qodana",
		Short: "Qodana is a fantastic code quality tool",
		Long:  "Abracadabra, abracadabra, abracadabra",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logLevel, err := log.ParseLevel(viper.GetString("log-level"))
			if err != nil {
				log.Fatal(err)
			}
			log.SetLevel(logLevel)
		},
		Run: func(cmd *cobra.Command, args []string) {
			err := pkg.Greet()
			if err != nil {
				log.Fatal(err)
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

func init() {
	RootCmd.AddCommand(
		linters.NewJsCommand(),
		linters.NewPhpCommand(),
		linters.NewPyCommand(),
		linters.NewGoCommand(),
		linters.NewJvmCommand(),
	)
}

func Execute() error {
	if err := RootCmd.Execute(); err != nil {
		return err
	}
	return nil
}
