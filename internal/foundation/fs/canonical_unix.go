//go:build unix

package fs

import (
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const maxSymlinkFollows = 255

// Canonical returns the canonical form of path: absolute, with all symlinks
// resolved, dot/dotdot segments eliminated, redundant separators removed,
// and filename case normalized on case-insensitive filesystems.
// The path must exist; returns an error otherwise.
func Canonical(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("canonical: empty path")
	}

	path, err := MakeAbsolute(path)
	if err != nil {
		return "", err
	}

	trailingSlash := len(path) > 1 && path[len(path)-1] == os.PathSeparator

	result, err := resolveImpl(path, false)
	if err != nil {
		return "", err
	}

	// A trailing slash implies the path must be a directory (POSIX behavior).
	if trailingSlash {
		info, err := os.Stat(result)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", &os.PathError{Op: "canonical", Path: path, Err: syscall.ENOTDIR}
		}
	}

	return result, nil
}

// WeaklyCanonical is like Canonical but does not require the full path to exist.
// It canonicalizes the longest existing prefix, then appends the remaining
// non-existent tail preserved as-is (no lexical cleaning).
func WeaklyCanonical(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("weakly canonical: empty path")
	}

	path, err := MakeAbsolute(path)
	if err != nil {
		return "", err
	}

	return resolveImpl(path, true)
}

// resolveImpl implements realpath-like component-by-component resolution.
// When weak is true, non-existent components are appended as-is instead of
// causing an error.
func resolveImpl(absPath string, weak bool) (string, error) {
	queue := splitPath(absPath)
	resolved := "/"
	symlinkCount := 0

	for len(queue) > 0 {
		comp := queue[0]
		queue = queue[1:]

		if comp == "" || comp == "." {
			continue
		}
		if comp == ".." {
			resolved = filepath.Dir(resolved)
			continue
		}

		// Look up the actual on-disk name (normalizes case on case-insensitive FS).
		actualName, err := findEntry(resolved, comp)
		if err != nil {
			if weak && errors.Is(err, os.ErrNotExist) {
				return joinTail(resolved, comp, queue), nil
			}
			return "", err
		}

		next := filepath.Join(resolved, actualName)

		info, err := os.Lstat(next)
		if err != nil {
			if weak && errors.Is(err, os.ErrNotExist) {
				return joinTail(resolved, comp, queue), nil
			}
			return "", err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			symlinkCount++
			if symlinkCount > maxSymlinkFollows {
				return "", &os.PathError{Op: "canonical", Path: absPath, Err: syscall.ELOOP}
			}

			target, err := os.Readlink(next)
			if err != nil {
				return "", err
			}

			// Prepend symlink target components before the remaining queue.
			queue = append(splitPath(target), queue...)

			if filepath.IsAbs(target) {
				resolved = "/"
			}
		} else {
			resolved = next
		}
	}

	return resolved, nil
}

// joinTail joins a resolved prefix, a current component, and any remaining
// queue components using the safe Join (no filepath.Clean).
func joinTail(resolved, current string, remaining []string) string {
	tail := Join(remaining...)
	return Join(resolved, current, tail)
}

// splitPath splits a path by the OS separator. Empty strings from leading
// or repeated separators are preserved; the caller skips them.
func splitPath(path string) []string {
	return strings.Split(path, string(os.PathSeparator))
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

// findEntry looks up a directory entry by name, returning the actual on-disk name.
// It uses Readdirnames for a single-pass scan, returning early on exact match
// (common on case-sensitive FS) or falling back to case-insensitive match.
func findEntry(dir, name string) (result string, err error) {
	d, err := os.Open(dir)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := d.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

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
