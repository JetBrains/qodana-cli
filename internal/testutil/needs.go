package testutil

import (
	"os"
	"strings"
	"testing"
)

// Flag represents a test prerequisite controlled via an environment variable.
// By default (env var unset), the flag is considered enabled so that all tests
// run locally without extra configuration. CI disables specific flags by
// setting them to a falsy value ("0", "false", or empty string).
type Flag struct {
	Name   string
	EnvVar string
}

var (
	Docker         = Flag{"Docker", "QT_ENABLE_DOCKER"}
	ContainerTests = Flag{"ContainerTests", "QT_ENABLE_CONTAINER_TESTS"}
	ClangDeps      = Flag{"ClangDeps", "QT_ENABLE_CLANG_DEPS"}
	CdnetDeps      = Flag{"CdnetDeps", "QT_ENABLE_CDNET_DEPS"}
)

// Need skips the test if any of the given flags are disabled.
func Need(t testing.TB, flags ...Flag) {
	t.Helper()
	for _, f := range flags {
		if !f.Enabled() {
			t.Skipf("skipping: requires %s (set %s=1 to enable)", f.Name, f.EnvVar)
		}
	}
}

// Enabled reports whether the flag is active. A flag is enabled when its
// environment variable is either unset or set to a truthy value.
func (f Flag) Enabled() bool {
	v, ok := os.LookupEnv(f.EnvVar)
	if !ok {
		return true
	}
	switch strings.ToLower(v) {
	case "", "0", "false":
		return false
	default:
		return true
	}
}
