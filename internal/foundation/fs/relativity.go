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
