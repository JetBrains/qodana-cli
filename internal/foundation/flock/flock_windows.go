//go:build windows

package flock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// With acquires an exclusive file lock on lockPath, runs fn, then releases the lock.
// The lock file is created if it doesn't exist. This provides inter-process mutual exclusion
// so that concurrent invocations of the same script can safely coordinate.
func With(lockPath string, fn func()) (err error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return fmt.Errorf("failed to create lock dir: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open lock file %s: %w", lockPath, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()

	const lockfileExclusiveLock = 0x02
	ol := &windows.Overlapped{}
	if err := windows.LockFileEx(windows.Handle(f.Fd()), lockfileExclusiveLock, 0, 1, 0, ol); err != nil {
		return fmt.Errorf("failed to acquire lock %s: %w", lockPath, err)
	}
	defer func() { err = errors.Join(err, windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)) }()

	fn()
	return nil
}
