//go:build !windows

package flock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// With acquires an exclusive POSIX file lock on lockPath, runs fn, then releases the lock.
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

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire lock %s: %w", lockPath, err)
	}
	defer func() { err = errors.Join(err, syscall.Flock(int(f.Fd()), syscall.LOCK_UN)) }()

	fn()
	return nil
}
