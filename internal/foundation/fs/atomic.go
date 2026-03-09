package fs

import (
	"fmt"
	"os"
)

// AtomicWriter writes to a temporary file and atomically renames it to the
// target path on Close. Call Abort to discard the temp file on error paths.
type AtomicWriter struct {
	path    string
	tmpPath string
	file    *os.File
}

// CreateAtomic creates a new atomic writer for the given path.
// Writes go to <path>.<pid>.temp; Close renames to the final path.
// The caller must call Close (to commit) or Abort (to discard).
func CreateAtomic(path string, perm os.FileMode) (*AtomicWriter, error) {
	tmpPath := fmt.Sprintf("%s.%d.temp", path, os.Getpid())
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return nil, fmt.Errorf("creating temp file %s: %w", tmpPath, err)
	}
	return &AtomicWriter{path: path, tmpPath: tmpPath, file: f}, nil
}

// Write implements io.Writer.
func (w *AtomicWriter) Write(p []byte) (int, error) {
	return w.file.Write(p)
}

// Close syncs, closes the temp file, and atomically renames it to the target.
func (w *AtomicWriter) Close() error {
	if err := w.file.Sync(); err != nil {
		w.file.Close()
		os.Remove(w.tmpPath)
		return fmt.Errorf("syncing %s: %w", w.tmpPath, err)
	}
	if err := w.file.Close(); err != nil {
		os.Remove(w.tmpPath)
		return fmt.Errorf("closing %s: %w", w.tmpPath, err)
	}
	if err := os.Rename(w.tmpPath, w.path); err != nil {
		os.Remove(w.tmpPath)
		return fmt.Errorf("renaming %s → %s: %w", w.tmpPath, w.path, err)
	}
	return nil
}

// Abort discards the temp file without renaming.
func (w *AtomicWriter) Abort() {
	w.file.Close()
	os.Remove(w.tmpPath)
}

// WriteFileAtomic writes data to path atomically via a temp file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	w, err := CreateAtomic(path, perm)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		w.Abort()
		return err
	}
	return w.Close()
}
