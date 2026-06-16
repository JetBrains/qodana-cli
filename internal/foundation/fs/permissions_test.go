//go:build darwin || linux

package fs

import (
	"os"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/testutil/needs"
)

// requireReadDirDenied asserts that the OS actually denies ReadDir on dir, which
// the caller has just chmodded to a restrictive mode (e.g. 0o111 or 0o000).
// Root and CAP_DAC_READ_SEARCH / CAP_DAC_OVERRIDE bypass discretionary access
// control, so ReadDir succeeds and the chmod-based test becomes meaningless.
// Fail loudly with an actionable hint instead of letting a later assertion fail
// with a misleading "expected error got nil".
func requireReadDirDenied(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.ReadDir(dir); err == nil {
		t.Fatalf("environment does not enforce restrictive directory permissions "+
			"(uid=%d, gid=%d); run as a non-root user, or set %s=0 to skip",
			os.Getuid(), os.Getgid(), needs.NonRoot.EnvVar)
	}
}
