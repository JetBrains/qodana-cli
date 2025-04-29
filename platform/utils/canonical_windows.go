package utils

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func Canonical(path string) (result string, err error) {
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

	return filepath.Clean(windows.UTF16ToString(utf16result)), nil
}
