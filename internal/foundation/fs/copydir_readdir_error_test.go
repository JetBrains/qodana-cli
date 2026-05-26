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

func TestCopyDir_ReadDirError(t *testing.T) {
	needs.Need(t, needs.NonRoot)

	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("data"), 0o644))

	require.NoError(t, os.Chmod(srcDir, 0o000))
	// Restore perms before t.TempDir's RemoveAll cleanup runs.
	// t.Cleanup is LIFO; this runs before the TempDir cleanup registered earlier.
	t.Cleanup(func() { _ = os.Chmod(srcDir, 0o755) })

	// Precondition: chmod 0o000 must actually deny ReadDir. Root or
	// CAP_DAC_READ_SEARCH bypass DAC and make the rest of the test
	// meaningless — surface that loudly instead of returning a misleading
	// "expected error got nil" later.
	if _, err := os.ReadDir(srcDir); err == nil {
		t.Fatalf("environment does not enforce 0o000 directory perms "+
			"(uid=%d, gid=%d); run as non-root user, or set %s=0 to skip",
			os.Getuid(), os.Getgid(), needs.NonRoot.EnvVar)
	}

	err := CopyDir(srcDir, filepath.Join(t.TempDir(), "dst"))
	assert.Error(t, err)
}
