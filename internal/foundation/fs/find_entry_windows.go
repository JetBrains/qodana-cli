package fs

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

// findEntry looks up a directory entry by name, returning the actual on-disk name.
// On Windows, it uses FindFirstFileW which returns the on-disk name regardless
// of 8.3 name generation settings, unlike the GetShortPathName/GetLongPathName
// roundtrip which silently fails when 8.3 names are disabled.
func findEntry(dir, name string) (string, error) {
	path := filepath.Join(dir, name)
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}

	var data windows.Win32finddata
	handle, err := windows.FindFirstFile(pathPtr, &data)
	if err != nil {
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) ||
			errors.Is(err, syscall.ERROR_PATH_NOT_FOUND) {
			return "", &os.PathError{Op: "canonical", Path: path, Err: os.ErrNotExist}
		}
		return "", err
	}
	// FindClose errors are extremely unlikely (kernel bug) and non-actionable.
	_ = windows.FindClose(handle)

	return windows.UTF16ToString(data.FileName[:]), nil
}
