package linters

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// GoCmd is an entry point for running go qodana linter
var GoCmd = &cobra.Command{
	Use:   "go",
	Short: "Qodana Go",
	Long:  "Qodana Go",
	Run: func(cmd *cobra.Command, args []string) {
		log.Info("To be implemented soon...")
	},
}
