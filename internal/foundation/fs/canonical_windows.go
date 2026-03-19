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

	path, err := MakeAbsolute(path)
	if err != nil {
		return "", err
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
// non-existent tail preserved as-is (no lexical cleaning).
func WeaklyCanonical(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("weakly canonical: empty path")
	}

	path, err := MakeAbsolute(path)
	if err != nil {
		return "", err
	}

	return weaklyCanonicalImpl(path, 0)
}

func weaklyCanonicalImpl(path string, depth int) (string, error) {
	if depth > maxSymlinkFollows {
		return "", &os.PathError{Op: "weakly canonical", Path: path, Err: syscall.ELOOP}
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
					return weaklyCanonicalImpl(target, depth+1)
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

// normalizeCaseWindows normalizes filename case on Windows by looking up
// each path component via FindFirstFileW, which returns the on-disk name
// regardless of 8.3 name generation settings.
func normalizeCaseWindows(absPath string) (string, error) {
	vol := filepath.VolumeName(absPath)
	if vol == "" {
		return absPath, nil
	}

	rest := absPath[len(vol):]
	components := strings.Split(rest, string(os.PathSeparator))

	built := strings.ToUpper(vol) + string(os.PathSeparator)

	for _, comp := range components {
		if comp == "" {
			continue
		}
		actual, err := findEntry(built, comp)
		if err != nil {
			return "", err
		}
		built = filepath.Join(built, actual)
	}

	return built, nil
}
