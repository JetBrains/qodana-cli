package linters

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// JsCmd is an entry point for running javascript qodana linter
var JsCmd = &cobra.Command{
	Use:   "js",
	Short: "Qodana JavaScript",
	Long:  "Qodana JavaScript",
	Run: func(cmd *cobra.Command, args []string) {
		log.Info("To be implemented soon...")
	},
}
