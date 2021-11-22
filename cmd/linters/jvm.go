package linters

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// JvmCmd is an entry point for running jvm qodana linter
var JvmCmd = &cobra.Command{
	Use:   "jvm",
	Short: "Qodana JVM",
	Long:  "Qodana JVM",
	Run: func(cmd *cobra.Command, args []string) {
		log.Info("To be implemented soon...")
	},
}
