package utils

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"os"

	"golang.org/x/sys/windows"
)

// CanonicalPath produces a standard, or "canonical" form of a path.
// A canonical path has the following properties:
// - is absolute;
// - contains no directory traversal segments such as `.` and `..`;
// - contains no symbolic links;
// - has normalized case, in filesystems which are case-insensitive;
// - contains no repeated path separators;
// - all path separators are backslashes.
// Since producing a canonical path implies resolution of symlinks, the path must exist to be canonicalized.
func CanonicalPath(path string) (result string, err error) {
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		// Do not use `filepath.Join`: it would call `Clean` on the result, breaking paths like `symlink/..`
		path = cwd + string(os.PathSeparator) + path
	}

	utf16path, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}

	kernel32Dll := windows.NewLazySystemDLL("kernel32.dll")
	GetShortPathNameW := kernel32Dll.NewProc("GetShortPathNameW")
	GetLongPathNameW := kernel32Dll.NewProc("GetLongPathNameW")

	// Convert to a short path
	requiredSize, _, err := GetShortPathNameW.Call(
		uintptr(unsafe.Pointer(utf16path)),
		uintptr(unsafe.Pointer(nil)),
		uintptr(0),
	)
	if err != nil && err != syscall.Errno(0) {
		return "", err
	}
	utf16shortPath := make([]uint16, requiredSize)

	_, _, err = GetShortPathNameW.Call(
		uintptr(unsafe.Pointer(utf16path)),
		uintptr(unsafe.Pointer(&utf16shortPath[0])),
		uintptr(requiredSize),
	)
	if err != nil && err != syscall.Errno(0) {
		return "", err
	}

	// Convert back to a long path
	requiredSize, _, err = GetLongPathNameW.Call(
		uintptr(unsafe.Pointer(&utf16shortPath[0])),
		uintptr(unsafe.Pointer(nil)),
		uintptr(0),
	)
	if err != nil && err != syscall.Errno(0) {
		return "", err
	}
	utf16result := make([]uint16, requiredSize)

	_, _, err = GetLongPathNameW.Call(
		uintptr(unsafe.Pointer(&utf16shortPath[0])),
		uintptr(unsafe.Pointer(&utf16result[0])),
		uintptr(requiredSize),
	)
	if err != nil && err != syscall.Errno(0) {
		return "", err
	}

	result, err = filepath.EvalSymlinks(windows.UTF16ToString(utf16result))
	if err != nil {
		return "", err
	}
	return result, err
}
