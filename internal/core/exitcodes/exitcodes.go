package exitcodes

import (
	"math"

	"github.com/JetBrains/qodana-cli/internal/foundation/exec"
)

const (
	// QodanaSuccessExitCode is Qodana exit code when the analysis is successfully completed.
	QodanaSuccessExitCode = 0
	// QodanaFailThresholdExitCode same as QodanaSuccessExitCode, but the threshold is set and exceeded.
	QodanaFailThresholdExitCode = 255
	// QodanaOutOfMemoryExitCode reports an interrupted process, sometimes because of an OOM.
	QodanaOutOfMemoryExitCode = exec.OomExitCode
	// QodanaEapLicenseExpiredExitCode reports an expired license.
	QodanaEapLicenseExpiredExitCode = 7
	// QodanaTimeoutExitCodePlaceholder is not a real exit code (it is not obtained from IDE process! and not returned from CLI)
	// Placeholder used to identify the case when the analysis reached timeout
	QodanaTimeoutExitCodePlaceholder = 1000
	// QodanaEmptyChangesetExitCodePlaceholder is not a real exit code (it is not obtained from IDE process! and not returned from CLI)
	// Placeholder used to identify the case when the changeset for scoped analysis is empty
	QodanaEmptyChangesetExitCodePlaceholder = 2000
	// QodanaInternalErrorExitCode is returned when the CLI itself fails (e.g. invalid arguments, failed to start process).
	// It is not a real process exit code. Use this to distinguish CLI errors from subprocess exit codes.
	// math.MinInt is chosen to never collide with real exit codes (0-255 on Unix, 0-65535 on Windows).
	QodanaInternalErrorExitCode = math.MinInt
)
