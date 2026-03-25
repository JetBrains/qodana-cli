//go:build unix && !darwin && !linux

package fs

// findEntry looks up a directory entry by name, returning the actual on-disk name.
// On platforms without dedicated OS APIs (not macOS or Linux), it delegates to
// the portable Readdirnames-based scan, which requires READ permission on the
// parent directory.
func findEntry(dir, name string) (string, error) {
	return findEntryByReaddir(dir, name)
}
