package fs

import (
	"os"
	"path/filepath"
)

// PathRelativity classifies how a path relates to the filesystem root.
// On Windows there are four categories (matching .NET terminology);
// on Unix only Absolute and Relative are possible.
type PathRelativity int

const (
	Relative      PathRelativity = iota // foo, ./bar, ../baz
	RootRelative                        // \foo, /foo (Windows: root of current drive)
	DriveRelative                       // C:foo (Windows: CWD on specified drive)
	Absolute                            // /foo (Unix), C:\foo (Windows), \\server\share (UNC)
)

func (r PathRelativity) String() string {
	switch r {
	case Relative:
		return "Relative"
	case RootRelative:
		return "RootRelative"
	case DriveRelative:
		return "DriveRelative"
	case Absolute:
		return "Absolute"
	default:
		return "Unknown"
	}
}

// MakeAbsolute converts any path to an absolute path without calling
// filepath.Clean (which would collapse symlink/.. lexically).
// For DriveRelative paths (C:foo), falls back to filepath.Abs since resolving
// the per-drive CWD requires the OS; this applies Clean but drive-relative
// paths with symlink/.. are vanishingly rare.
func MakeAbsolute(path string) (string, error) {
	switch Relativity(path) {
	case Absolute:
		return path, nil
	case RootRelative:
		// \Users → C:\Users (prepend current drive volume).
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.VolumeName(cwd) + path, nil
	case DriveRelative:
		// C:foo → resolve via filepath.Abs (calls Windows GetFullPathName
		// which knows each drive's CWD). This applies Clean, but
		// drive-relative paths with symlink/.. are vanishingly rare.
		return filepath.Abs(path) //nolint:forbidigo // needed for per-drive CWD resolution
	default: // Relative
		// Do not use filepath.Join: it calls Clean, which collapses
		// symlink/.. lexically.
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return cwd + string(os.PathSeparator) + path, nil
	}
}

// Join joins path elements with the OS separator without calling filepath.Clean.
// Unlike filepath.Join, this preserves symlink/.. semantics.
// Empty elements are skipped.
func Join(elem ...string) string {
	var b []byte
	for _, e := range elem {
		if e == "" {
			continue
		}
		if len(b) > 0 && !os.IsPathSeparator(b[len(b)-1]) {
			b = append(b, os.PathSeparator)
		}
		b = append(b, e...)
	}
	if len(b) == 0 {
		return ""
	}
	// Normalize separators on Windows (forward slash → backslash).
	for i := range b {
		if b[i] == '/' && os.PathSeparator == '\\' {
			b[i] = '\\'
		}
	}
	// Collapse runs of separators (but preserve UNC prefix \\server).
	out := b[:0]
	for i := 0; i < len(b); i++ {
		if os.IsPathSeparator(b[i]) && len(out) > 0 && os.IsPathSeparator(out[len(out)-1]) {
			if len(out) <= 1 {
				out = append(out, b[i])
				continue
			}
			continue
		}
		out = append(out, b[i])
	}
	// Remove trailing separator unless it's the root.
	result := string(out)
	if len(result) > 1 && os.IsPathSeparator(result[len(result)-1]) {
		vol := filepath.VolumeName(result)
		if len(result) > len(vol)+1 {
			result = result[:len(result)-1]
		}
	}
	return result
}

// Dir returns the parent directory of path without calling filepath.Clean.
// Unlike filepath.Dir, this does not lexically resolve "." or ".." components.
// On an already-canonical path, the behavior is identical to filepath.Dir.
func Dir(path string) string {
	vol := filepath.VolumeName(path)
	rest := path[len(vol):]

	// Find last separator.
	i := len(rest) - 1
	for i >= 0 && !os.IsPathSeparator(rest[i]) {
		i--
	}
	if i < 0 {
		if vol != "" {
			return vol + "."
		}
		return "."
	}
	// Trim trailing separators from the directory part (but keep root).
	dir := rest[:i]
	for len(dir) > 0 && os.IsPathSeparator(dir[len(dir)-1]) {
		dir = dir[:len(dir)-1]
	}
	if dir == "" {
		return vol + string(os.PathSeparator)
	}
	return vol + dir
}

// Relativity classifies a path into one of the four categories.
func Relativity(path string) PathRelativity {
	if filepath.IsAbs(path) {
		return Absolute
	}
	vol := filepath.VolumeName(path)
	rest := path[len(vol):]
	if vol != "" {
		// Has volume but not absolute → drive-relative (C:foo).
		return DriveRelative
	}
	if len(rest) > 0 && os.IsPathSeparator(rest[0]) {
		// No volume but starts with separator → root-relative (\foo).
		return RootRelative
	}
	return Relative
}
