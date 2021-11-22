package linters

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// PhpCmd is an entry point for running php qodana linter
var PhpCmd = &cobra.Command{
	Use:   "php",
	Short: "Qodana JavaScript",
	Long:  "Qodana JavaScript",
	Run: func(cmd *cobra.Command, args []string) {
		log.Info("To be implemented soon...")
	},
}
