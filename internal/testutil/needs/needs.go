package needs

import (
	"os"
	"testing"
)

// Flag represents a test prerequisite controlled via an environment variable.
// By default (env var unset or empty), the flag is considered enabled so that
// all tests run locally without extra configuration. CI disables specific flags
// by setting them to "0".
type Flag struct {
	Name   string
	EnvVar string
}

var (
	Docker         = Flag{Name: "Docker", EnvVar: "QT_ENABLE_DOCKER"}
	ContainerTests = Flag{Name: "ContainerTests", EnvVar: "QT_ENABLE_CONTAINER_TESTS"}
	ClangDeps      = Flag{Name: "ClangDeps", EnvVar: "QT_ENABLE_CLANG_DEPS"}
	CdnetDeps      = Flag{Name: "CdnetDeps", EnvVar: "QT_ENABLE_CDNET_DEPS"}
	CasefoldFS     = Flag{Name: "CasefoldFS", EnvVar: "QT_ENABLE_CASEFOLD_FS"}
)

// Need skips the test if any of the given flags are disabled. OS restrictions are expressed by
// build-constraining the test file (e.g. //go:build linux), not here: qodana-clang/qodana-cdnet
// linter tests live in linux-only files.
func Need(t testing.TB, flags ...Flag) {
	t.Helper()
	for _, f := range flags {
		if !f.check(t) {
			t.Skipf("skipping: requires %s (set %s=1 to enable)", f.Name, f.EnvVar)
		}
	}
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
