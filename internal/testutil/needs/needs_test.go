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

// TestFlagSpellings pins each public flag's Name and EnvVar so a typo in the
// var declaration shows up here rather than in a downstream test that
// silently no-ops because the env var name doesn't match what CI sets.
func TestFlagSpellings(t *testing.T) {
	for _, tt := range []struct {
		got            Flag
		wantName       string
		wantEnvVar     string
	}{
		{Docker, "Docker", "QT_ENABLE_DOCKER"},
		{ContainerTests, "ContainerTests", "QT_ENABLE_CONTAINER_TESTS"},
		{ClangDeps, "ClangDeps", "QT_ENABLE_CLANG_DEPS"},
		{CdnetDeps, "CdnetDeps", "QT_ENABLE_CDNET_DEPS"},
		{CasefoldFS, "CasefoldFS", "QT_ENABLE_CASEFOLD_FS"},
		{NonRoot, "NonRoot", "QT_ENABLE_NON_ROOT"},
	} {
		t.Run(tt.wantName, func(t *testing.T) {
			assert.Equal(t, tt.wantName, tt.got.Name)
			assert.Equal(t, tt.wantEnvVar, tt.got.EnvVar)
		})
	}
}
