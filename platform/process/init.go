package process

import (
	"github.com/JetBrains/qodana-cli/v2025/core"
	"github.com/JetBrains/qodana-cli/v2025/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/JetBrains/qodana-cli/v2025/platform/version"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// Init runs miscellaneous process-wide utility code.
func Init() {
	KillProcessTreeOnClose()

	commoncontext.InterruptChannel = make(chan os.Signal, 1)
	signal.Notify(commoncontext.InterruptChannel, os.Interrupt)
	signal.Notify(commoncontext.InterruptChannel, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-commoncontext.InterruptChannel
		msg.WarningMessage("Interrupting Qodana...")
		log.SetOutput(io.Discard)
		core.CheckForUpdates(version.Version)
		core.ContainerCleanup()
		_ = msg.QodanaSpinner.Stop()
		os.Exit(0)
	}()
}
