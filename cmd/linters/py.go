package linters

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// PyCmd is an entry point for running php qodana linter
var PyCmd = &cobra.Command{
	Use:   "py",
	Short: "Qodana Python",
	Long:  "Qodana Python",
	Run: func(cmd *cobra.Command, args []string) {
		log.Info("To be implemented soon...")
	},
}
