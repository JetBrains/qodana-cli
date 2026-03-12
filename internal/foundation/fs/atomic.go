package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriter writes to a temporary file and atomically renames it to the
// target path on Close. Call Abort to discard the temp file on error paths.
type AtomicWriter struct {
	path    string
	tmpPath string
	file    *os.File
}

// CreateAtomic creates a new atomic writer for the given path.
// Writes go to a unique temp file in the same directory; Close renames to the final path.
// Safe for concurrent use: each call gets its own temp file.
// The caller must call Close (to commit) or Abort (to discard).
func CreateAtomic(path string, perm os.FileMode) (*AtomicWriter, error) {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.temp")
	if err != nil {
		return nil, fmt.Errorf("creating temp file for %s: %w", path, err)
	}
	if err := f.Chmod(perm); err != nil {
		return nil, errors.Join(fmt.Errorf("setting permissions on %s: %w", f.Name(), err),
			f.Close(), os.Remove(f.Name()))
	}
	return &AtomicWriter{path: path, tmpPath: f.Name(), file: f}, nil
}

// Write implements io.Writer.
func (w *AtomicWriter) Write(p []byte) (int, error) {
	return w.file.Write(p)
}

// Close syncs, closes the temp file, and atomically renames it to the target.
func (w *AtomicWriter) Close() error {
	if err := w.file.Sync(); err != nil {
		return errors.Join(fmt.Errorf("syncing %s: %w", w.tmpPath, err), w.cleanup())
	}
	if err := w.file.Close(); err != nil {
		return errors.Join(fmt.Errorf("closing %s: %w", w.tmpPath, err), os.Remove(w.tmpPath))
	}
	if err := os.Rename(w.tmpPath, w.path); err != nil {
		return errors.Join(fmt.Errorf("renaming %s → %s: %w", w.tmpPath, w.path, err), os.Remove(w.tmpPath))
	}
	return nil
}

// Abort discards the temp file without renaming.
func (w *AtomicWriter) Abort() error {
	return w.cleanup()
}

func (w *AtomicWriter) cleanup() error {
	return errors.Join(w.file.Close(), os.Remove(w.tmpPath))
}

// WriteFileAtomic writes data to path atomically via a temp file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	w, err := CreateAtomic(path, perm)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return errors.Join(err, w.Abort())
	}
	return w.Close()
}
