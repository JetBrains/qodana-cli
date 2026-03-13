//go:build darwin

package fs

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// getattrlist syscall number on macOS (both amd64 and arm64).
// Defined locally to avoid the deprecated unix.SYS_GETATTRLIST constant.
const sysGetattrlist = 220

// findEntry looks up a directory entry by name, returning the actual on-disk name.
// On macOS, it uses getattrlist(2) with ATTR_CMN_NAME to retrieve the stored
// name from the directory entry. This only requires EXECUTE permission on the
// parent directory, not READ.
func findEntry(dir, name string) (string, error) {
	path := filepath.Join(dir, name)

	attrList := unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr:  unix.ATTR_CMN_NAME,
	}

	// Buffer layout returned by getattrlist with ATTR_CMN_NAME:
	//   bytes 0-3:            uint32 totalLength
	//   bytes 4-7:            int32  attr_dataoffset (relative to byte 4)
	//   bytes 8-11:           uint32 attr_length (includes null terminator)
	//   bytes 4+offset .. :   null-terminated name string
	buf := make([]byte, 4+8+256)

	if err := getattrlistCall(path, &attrList, buf, unix.FSOPT_NOFOLLOW); err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return "", &os.PathError{Op: "canonical", Path: path, Err: os.ErrNotExist}
		}
		return "", err
	}

	totalLen := binary.NativeEndian.Uint32(buf[0:4])
	if totalLen < 12 {
		return "", &os.PathError{Op: "canonical", Path: path, Err: syscall.EINVAL}
	}

	nameOffset := binary.NativeEndian.Uint32(buf[4:8])
	nameLength := binary.NativeEndian.Uint32(buf[8:12])

	// nameOffset is relative to the attrreference_t field (byte 4 in buffer).
	nameStart := 4 + int(nameOffset)
	// nameLength includes the null terminator.
	nameEnd := nameStart + int(nameLength) - 1

	if nameStart < 0 || nameEnd > len(buf) || nameStart >= nameEnd {
		return "", &os.PathError{Op: "canonical", Path: path, Err: syscall.EINVAL}
	}

	return string(buf[nameStart:nameEnd]), nil
}

func getattrlistCall(path string, attrList *unix.Attrlist, buf []byte, options uint32) error {
	pathPtr, err := unix.BytePtrFromString(path)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall6(
		sysGetattrlist,
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(attrList)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(options),
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
