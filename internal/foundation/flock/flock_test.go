package flock_test

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JetBrains/qodana-cli/internal/foundation/flock"
)

func TestWith_MutualExclusion(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	var running atomic.Int32
	var maxConcurrent atomic.Int32
	var wg sync.WaitGroup

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := flock.With(lockPath, func() {
				cur := running.Add(1)
				// Track the maximum number of concurrent executions
				for {
					old := maxConcurrent.Load()
					if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
				running.Add(-1)
			})
			if err != nil {
				t.Errorf("flock.With failed: %v", err)
			}
		}()
	}

	wg.Wait()

	if max := maxConcurrent.Load(); max != 1 {
		t.Errorf("expected max 1 concurrent execution, got %d", max)
	}
}

func TestWith_CreatesLockFile(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "sub", "dir", "test.lock")

	err := flock.With(lockPath, func() {
		if _, err := os.Stat(lockPath); err != nil {
			t.Errorf("lock file should exist during callback: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("flock.With failed: %v", err)
	}
}

func TestWith_CallbackExecutes(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")
	called := false

	err := flock.With(lockPath, func() {
		called = true
	})
	if err != nil {
		t.Fatalf("flock.With failed: %v", err)
	}
	if !called {
		t.Error("callback was not executed")
	}
}
