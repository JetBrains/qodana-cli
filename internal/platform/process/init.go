package process

import (
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JetBrains/qodana-cli/internal/core"
	"github.com/JetBrains/qodana-cli/internal/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
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
		core.PrintUpdateNotice()
		core.ContainerCleanup()
		_ = msg.QodanaSpinner.Stop()
		// Sleep for a second to allow other functions monitoring signals elsewhere to do their thing.
		// A future rewrite of the subprocess API should incorporate a more structured signal handling.
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
}
