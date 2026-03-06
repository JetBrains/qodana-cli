package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

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
		// Do not use filepath.Join: it calls Clean, which collapses symlink/.. lexically.
		path = cwd + string(os.PathSeparator) + path
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}

	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}

	return normalizeCaseWindows(resolved)
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

	// Try the full path first — fast path when everything exists.
	result, err := filepath.EvalSymlinks(path)
	if err == nil {
		result, err = filepath.Abs(result)
		if err == nil {
			return normalizeCaseWindows(result)
		}
	}

	// Walk components to find the longest existing prefix.
	vol := filepath.VolumeName(path)
	rest := path[len(vol):]
	components := strings.Split(rest, string(os.PathSeparator))
	sep := string(os.PathSeparator)

	built := vol + sep

	for i, comp := range components {
		if comp == "" || comp == "." {
			continue
		}
		if comp == ".." {
			built = filepath.Dir(built)
			continue
		}
		next := filepath.Join(built, comp)

		// Resolve symlinks for existing components so that the canonical
		// prefix reflects the real target, not the symlink name.
		resolved, evalErr := filepath.EvalSymlinks(next)
		if evalErr != nil {
			// EvalSymlinks failed — either the component doesn't exist, or
			// it's a symlink whose target is missing (broken symlink).
			// If it's a symlink, read its target and treat the result as
			// a non-existent path from the current prefix.
			if info, lstatErr := os.Lstat(next); lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
				target, readErr := os.Readlink(next)
				if readErr == nil {
					if !filepath.IsAbs(target) {
						target = filepath.Join(built, target)
					}
					tail := strings.Join(components[i+1:], sep)
					if tail != "" {
						target = target + sep + tail
					}
					return WeaklyCanonical(target)
				}
			}
			// Component doesn't exist — canonicalize what we have and append the rest.
			canonical, canonErr := normalizeCaseWindows(built)
			if canonErr != nil {
				canonical = built
			}
			tail := strings.Join(components[i:], sep)
			if strings.HasSuffix(canonical, sep) {
				return canonical + tail, nil
			}
			return canonical + sep + tail, nil
		}
		built = resolved
	}

	// Everything existed — normalize case.
	return normalizeCaseWindows(built)
}

// normalizeCaseWindows normalizes filename case on Windows using the
// GetShortPathName -> GetLongPathName round-trip. This forces Windows to
// return the actual on-disk casing for each path component.
func normalizeCaseWindows(absPath string) (string, error) {
	if _, err := os.Lstat(absPath); err != nil {
		return "", err
	}

	utf16Path, err := windows.UTF16PtrFromString(absPath)
	if err != nil {
		return "", err
	}

	// Get short path name.
	shortSize, err := windows.GetShortPathName(utf16Path, nil, 0)
	if err != nil && err != syscall.Errno(0) {
		return "", err
	}
	shortBuf := make([]uint16, shortSize)
	_, err = windows.GetShortPathName(utf16Path, &shortBuf[0], shortSize)
	if err != nil && err != syscall.Errno(0) {
		return "", err
	}

	// Convert back to long path name (this returns proper casing).
	longSize, err := windows.GetLongPathName(&shortBuf[0], nil, 0)
	if err != nil && err != syscall.Errno(0) {
		return "", err
	}
	longBuf := make([]uint16, longSize)
	_, err = windows.GetLongPathName(&shortBuf[0], &longBuf[0], longSize)
	if err != nil && err != syscall.Errno(0) {
		return "", err
	}

	return windows.UTF16ToString(longBuf), nil
}
