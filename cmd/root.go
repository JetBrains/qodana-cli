package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tiulpin/qodana/pkg"
)

// NewRootCmd constructs root command
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "qodana",
		Short:   "Run Qodana CLI",
		Long:    "Run Qodana CLI. Qodana is a code quality monitoring platform. Docs: https://jb.gg/qodana-docs",
		Version: "0.2.0",
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
		PostRun: func(cmd *cobra.Command, args []string) {
			pkg.Primary.Println("\n") // for proper output end in ZSH
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
