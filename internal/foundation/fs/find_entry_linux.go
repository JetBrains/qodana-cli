//go:build linux

package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// findEntry looks up a directory entry by name, returning the actual on-disk name.
// On Linux, it opens the entry with O_PATH|O_NOFOLLOW (which requires only EXECUTE
// on the parent directory, not READ) and reads the canonical path from /proc/self/fd.
// The kernel populates the dentry with the on-disk name, so readlink returns the
// correct casing on case-insensitive filesystems (ext4 casefold, VFAT).
//
// If readlink on /proc/self/fd fails (e.g. /proc unavailable or restricted),
// it falls back to the portable Readdirnames-based scan, which requires READ
// permission on the parent directory.
func findEntry(dir, name string) (string, error) {
	path := filepath.Join(dir, name)

	fd, err := unix.Open(path, unix.O_PATH|unix.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return "", &os.PathError{Op: "canonical", Path: path, Err: os.ErrNotExist}
		}
		return "", err
	}

	realPath, readlinkErr := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
	closeErr := unix.Close(fd)
	if readlinkErr != nil {
		return findEntryByReaddir(dir, name)
	}
	if closeErr != nil {
		return "", closeErr
	}

	return filepath.Base(realPath), nil
}
