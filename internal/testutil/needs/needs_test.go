package needs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlag_Check(t *testing.T) {
	f := Flag{Name: "Test", EnvVar: "QT_ENABLE_TEST_DUMMY"}

	tests := []struct {
		name    string
		set     bool
		value   string
		enabled bool
	}{
		{"unset is enabled", false, "", true},
		{"empty string is enabled", true, "", true},
		{"zero is disabled", true, "0", false},
		{"one is enabled", true, "1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(f.EnvVar, tt.value)
			}
			assert.Equal(t, tt.enabled, f.check(t))
		})
	}
}

func TestNeed_SkipsWhenDisabled(t *testing.T) {
	f := Flag{Name: "Fake", EnvVar: "QT_ENABLE_FAKE_NEED"}
	t.Setenv(f.EnvVar, "0")

	// We can't easily test t.Skip on a real *testing.T from inside another test,
	// so we verify via check() — the Skip path is a trivial wrapper.
	assert.False(t, f.check(t))
}

func TestNeed_RunsWhenEnabled(t *testing.T) {
	f := Flag{Name: "Fake", EnvVar: "QT_ENABLE_FAKE_NEED"}
	// env var unset → enabled; Need should not skip
	Need(t, f)
	// if we reach here, Need did not skip
}

func TestFlag_AvailableOn(t *testing.T) {
	// clang/cdnet linters only run in a Linux container, so their deps flags are linux-only.
	for _, f := range []Flag{ClangDeps, CdnetDeps} {
		assert.True(t, f.availableOn("linux"), "%s should be available on linux", f.Name)
		assert.False(t, f.availableOn("darwin"), "%s should not be available on darwin", f.Name)
		assert.False(t, f.availableOn("windows"), "%s should not be available on windows", f.Name)
	}
	// Platform-agnostic flags are available on every OS.
	for _, f := range []Flag{Docker, ContainerTests, CasefoldFS} {
		for _, goos := range []string{"linux", "darwin", "windows"} {
			assert.True(t, f.availableOn(goos), "%s should be available on %s", f.Name, goos)
		}
	}
}
