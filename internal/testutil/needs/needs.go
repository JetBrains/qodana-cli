package needs

import (
	"os"
	"runtime"
	"testing"
)

// Flag represents a test prerequisite controlled via an environment variable.
// By default (env var unset or empty), the flag is considered enabled so that
// all tests run locally without extra configuration. CI disables specific flags
// by setting them to "0".
type Flag struct {
	Name   string
	EnvVar string
	// GOOS, when non-empty, restricts the prerequisite to that operating system: on any other
	// GOOS the flag is unavailable and Need skips the test regardless of the env var. qodana-clang
	// and qodana-cdnet only run inside a Linux container, so their deps flags are linux-only.
	GOOS string
}

var (
	Docker         = Flag{Name: "Docker", EnvVar: "QT_ENABLE_DOCKER"}
	ContainerTests = Flag{Name: "ContainerTests", EnvVar: "QT_ENABLE_CONTAINER_TESTS"}
	ClangDeps      = Flag{Name: "ClangDeps", EnvVar: "QT_ENABLE_CLANG_DEPS", GOOS: "linux"}
	CdnetDeps      = Flag{Name: "CdnetDeps", EnvVar: "QT_ENABLE_CDNET_DEPS", GOOS: "linux"}
	CasefoldFS     = Flag{Name: "CasefoldFS", EnvVar: "QT_ENABLE_CASEFOLD_FS"}
)

// Need skips the test if any of the given flags are unavailable on this OS or disabled.
func Need(t testing.TB, flags ...Flag) {
	t.Helper()
	for _, f := range flags {
		if !f.availableOn(runtime.GOOS) {
			t.Skipf("skipping: %s only runs on %s (current OS: %s)", f.Name, f.GOOS, runtime.GOOS)
		}
		if !f.check(t) {
			t.Skipf("skipping: requires %s (set %s=1 to enable)", f.Name, f.EnvVar)
		}
	}
}

// availableOn reports whether the flag's prerequisite can exist on goos. An empty GOOS is
// platform-agnostic and available everywhere.
func (f Flag) availableOn(goos string) bool {
	return f.GOOS == "" || f.GOOS == goos
}

// check reports whether the flag is active and fatals on invalid values.
// A flag is enabled when its environment variable is unset, empty, or "1".
// It is disabled when set to "0". Any other value is a configuration error.
func (f Flag) check(t testing.TB) bool {
	t.Helper()
	v, ok := os.LookupEnv(f.EnvVar)
	if !ok || v == "" {
		return true
	}
	switch v {
	case "1":
		return true
	case "0":
		return false
	default:
		t.Fatalf("%s=%q is invalid: expected \"0\" or \"1\"", f.EnvVar, v)
		return false
	}
}
