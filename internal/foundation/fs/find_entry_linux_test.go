//go:build linux

package fs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/JetBrains/qodana-cli/internal/testutil/dockertest"
	"github.com/JetBrains/qodana-cli/internal/testutil/needs"
)

// TestFindEntry_CaseInsensitiveLinux validates that findEntry returns the on-disk
// name (not the input casing) on a case-insensitive Linux filesystem.
//
// The test re-executes itself inside a Docker container with CAP_SYS_ADMIN
// (needed to mount a VFAT image). Inside the container it creates a VFAT
// filesystem and verifies that findEntry normalizes case correctly.
//
// Needs: Docker, CasefoldFS.
func TestFindEntry_CaseInsensitiveLinux(t *testing.T) {
	needs.Need(t, needs.Docker, needs.CasefoldFS)
	if !dockertest.ReexecInDocker(t, "testdata/casefold-test/compose.yaml") {
		return
	}

	tmp := t.TempDir()
	img := filepath.Join(tmp, "fat.img")
	mnt := filepath.Join(tmp, "mnt")
	require.NoError(t, os.MkdirAll(mnt, 0o755))

	run(t, "dd", "if=/dev/zero", "of="+img, "bs=1M", "count=10")
	run(t, "mkfs.vfat", img)
	run(t, "mount", img, mnt)
	t.Cleanup(func() { _ = exec.Command("umount", mnt).Run() })

	require.NoError(t, os.MkdirAll(filepath.Join(mnt, "TestDir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mnt, "TestDir", "MixedCase"), []byte("x"), 0o644))

	t.Run("file wrong case", func(t *testing.T) {
		got, err := findEntry(filepath.Join(mnt, "TestDir"), "mixedcase")
		require.NoError(t, err)
		assert.Equal(t, "MixedCase", got, "findEntry must return on-disk casing")
	})

	t.Run("dir wrong case", func(t *testing.T) {
		got, err := findEntry(mnt, "testdir")
		require.NoError(t, err)
		assert.Equal(t, "TestDir", got, "findEntry must return on-disk casing for directories")
	})
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}
