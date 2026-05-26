//go:build darwin || linux

package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/testutil/needs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireExecOnlyEnforced verifies that chmodding `dir` to 0o111 (execute-only)
// actually denies ReadDir. Root or CAP_DAC_READ_SEARCH bypass DAC and make these
// tests meaningless — surface that loudly with an actionable opt-out hint.
func requireExecOnlyEnforced(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.ReadDir(dir); err == nil {
		t.Fatalf("environment does not enforce 0o111 directory perms "+
			"(uid=%d, gid=%d); run as non-root user, or set %s=0 to skip",
			os.Getuid(), os.Getgid(), needs.NonRoot.EnvVar)
	}
}

func TestCanonical_ExecOnlyDir(t *testing.T) {
	needs.Need(t, needs.NonRoot)

	tmp := canonicalTempDir(t)
	dir := filepath.Join(tmp, "execonly")
	mkdirp(t, dir)
	touch(t, filepath.Join(dir, "file"))

	require.NoError(t, os.Chmod(dir, 0o111))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	requireExecOnlyEnforced(t, dir)

	got, err := Canonical(filepath.Join(dir, "file"))
	require.NoError(t, err, "Canonical must work with execute-only parent dir")
	assert.Equal(t, filepath.Join(dir, "file"), got)
}

func TestWeaklyCanonical_ExecOnlyDir(t *testing.T) {
	needs.Need(t, needs.NonRoot)

	tmp := canonicalTempDir(t)
	dir := filepath.Join(tmp, "execonly")
	mkdirp(t, dir)
	touch(t, filepath.Join(dir, "file"))

	require.NoError(t, os.Chmod(dir, 0o111))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	requireExecOnlyEnforced(t, dir)

	got, err := WeaklyCanonical(filepath.Join(dir, "file"))
	require.NoError(t, err, "WeaklyCanonical must work with execute-only parent dir")
	assert.Equal(t, filepath.Join(dir, "file"), got)
}

func TestCanonical_ExecOnlyDirCaseNormalization(t *testing.T) {
	skipIfCaseSensitive(t)
	needs.Need(t, needs.NonRoot)

	tmp := canonicalTempDir(t)
	dir := filepath.Join(tmp, "ExecOnly")
	mkdirp(t, dir)
	touch(t, filepath.Join(dir, "MixedCase"))

	require.NoError(t, os.Chmod(dir, 0o111))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	requireExecOnlyEnforced(t, dir)

	// On a case-insensitive FS, asking for wrong-case "mixedcase" should
	// still return the on-disk name "MixedCase".
	got, err := Canonical(filepath.Join(tmp, "execonly", "mixedcase"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "ExecOnly", "MixedCase"), got)
}
