//go:build !windows

package fs

import (
	"fmt"
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

	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		path = cwd + string(os.PathSeparator) + path
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
// non-existent tail (cleaned of . and .. segments).
func WeaklyCanonical(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("weakly canonical: empty path")
	}

	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		path = cwd + string(os.PathSeparator) + path
	}

	return resolveImpl(path, true)
}

// resolveImpl implements realpath-like component-by-component resolution.
// When weak is true, non-existent components are appended cleaned instead of
// causing an error.
func resolveImpl(absPath string, weak bool) (string, error) {
	components := splitPath(absPath)
	resolved := "/"
	symlinkCount := 0

	for i := 0; i < len(components); i++ {
		comp := components[i]

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
			if weak && os.IsNotExist(err) {
				return appendTail(resolved, comp, components[i+1:]), nil
			}
			return "", err
		}

		next := filepath.Join(resolved, actualName)

		info, err := os.Lstat(next)
		if err != nil {
			if weak && os.IsNotExist(err) {
				return appendTail(resolved, comp, components[i+1:]), nil
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

			// Prepend remaining components after the symlink target.
			remaining := components[i+1:]
			targetComponents := splitPath(target)
			components = append(targetComponents, remaining...)
			i = -1 // restart loop (i++ will make it 0)

			if filepath.IsAbs(target) {
				resolved = "/"
			}
			// If relative, resolved stays at current built path.
		} else {
			resolved = next
		}
	}

	return resolved, nil
}

// appendTail joins the resolved prefix with the current component and any
// remaining components. The tail is kept as-is (no Clean) because unresolved
// components might be symlinks, making lexical .. collapsing incorrect.
func appendTail(resolved, current string, remaining []string) string {
	sep := string(os.PathSeparator)
	var b strings.Builder
	b.WriteString(resolved)
	if resolved != "/" {
		b.WriteString(sep)
	}
	b.WriteString(current)
	for _, r := range remaining {
		if r == "" {
			continue
		}
		b.WriteString(sep)
		b.WriteString(r)
	}
	return b.String()
}

// splitPath splits a path into its components, removing empty strings
// from repeated separators but preserving them for processing.
func splitPath(path string) []string {
	return strings.Split(path, string(os.PathSeparator))
}

// findEntry looks up a directory entry by name with case-insensitive fallback.
// On case-sensitive filesystems this finds an exact match; on case-insensitive
// ones it returns the actual on-disk name.
func findEntry(dir, name string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	lowerName := strings.ToLower(name)
	for _, e := range entries {
		if strings.ToLower(e.Name()) == lowerName {
			return e.Name(), nil
		}
	}

	return "", &os.PathError{
		Op:   "canonical",
		Path: filepath.Join(dir, name),
		Err:  os.ErrNotExist,
	}
}
