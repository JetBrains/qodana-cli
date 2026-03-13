//go:build unix && !darwin && !linux

package fs

import (
	"errors"
	"iter"
	"os"
	"path/filepath"
	"strings"
)

// findEntry looks up a directory entry by name, returning the actual on-disk name.
// On platforms without dedicated OS APIs (not macOS or Linux), it falls back to
// scanning directory entries via Readdirnames — this requires READ permission on
// the parent directory.
func findEntry(dir, name string) (result string, err error) {
	d, err := os.Open(dir)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, d.Close()) }()

	lowerName := strings.ToLower(name)
	caseInsensitiveMatch := ""

	for n := range readdirnames(d) {
		if n == name {
			return n, nil // exact match — return immediately
		}
		if caseInsensitiveMatch == "" && strings.ToLower(n) == lowerName {
			caseInsensitiveMatch = n
		}
	}

	if caseInsensitiveMatch != "" {
		// Only accept a case-insensitive match if the filesystem actually
		// treats them as equivalent. On a case-sensitive FS, Lstat with the
		// requested (possibly wrong-case) name will fail.
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			return caseInsensitiveMatch, nil
		}
	}

	return "", &os.PathError{
		Op:   "canonical",
		Path: filepath.Join(dir, name),
		Err:  os.ErrNotExist,
	}
}

// readdirnames returns an iterator over all entry names in an open directory,
// reading in batches of 256 per syscall.
func readdirnames(d *os.File) iter.Seq[string] {
	return func(yield func(string) bool) {
		for {
			names, err := d.Readdirnames(256)
			for _, n := range names {
				if !yield(n) {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
}
