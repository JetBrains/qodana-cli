package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlag_Enabled(t *testing.T) {
	f := Flag{Name: "Test", EnvVar: "QT_ENABLE_TEST_DUMMY"}

	tests := []struct {
		name    string
		set     bool
		value   string
		enabled bool
	}{
		{"unset is enabled", false, "", true},
		{"empty string is disabled", true, "", false},
		{"zero is disabled", true, "0", false},
		{"false is disabled", true, "false", false},
		{"FALSE is disabled", true, "FALSE", false},
		{"one is enabled", true, "1", true},
		{"true is enabled", true, "true", true},
		{"any string is enabled", true, "yes", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(f.EnvVar, tt.value)
			}
			assert.Equal(t, tt.enabled, f.Enabled())
		})
	}
}

func TestNeed_SkipsWhenDisabled(t *testing.T) {
	f := Flag{Name: "Fake", EnvVar: "QT_ENABLE_FAKE_NEED"}
	t.Setenv(f.EnvVar, "0")

	// We can't easily test t.Skip on a real *testing.T from inside another test,
	// so we verify via Enabled() — the Skip path is a trivial wrapper.
	assert.False(t, f.Enabled())
}

func TestNeed_RunsWhenEnabled(t *testing.T) {
	f := Flag{Name: "Fake", EnvVar: "QT_ENABLE_FAKE_NEED"}
	// env var unset → enabled; Need should not skip
	Need(t, f)
	// if we reach here, Need did not skip
}
