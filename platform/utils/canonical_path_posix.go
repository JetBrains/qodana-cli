//go:build unix

package utils

// #include <stdlib.h>
import "C"
import (
	"unsafe"
)

// CanonicalPath produces a standard, or "canonical" form of a path.
// A canonical path has the following properties:
// - is absolute;
// - contains no directory traversal segments such as `.` and `..`;
// - contains no symbolic links;
// - has normalized case, in filesystems which are case-insensitive;
// - contains no repeated path separators.
// Since producing a canonical path implies resolution of symlinks, the path must exist to be canonicalized.
// On POSIX systems, this function calls `realpath`.
// See also: https://www.man7.org/linux/man-pages/man3/realpath.3.html
func CanonicalPath(path string) (result string, err error) {
	// Note that emulating `realpath` using standard Go functions such as `Abs` and `EvalSymlinks` is not enough,
	// because these functions do nothing to normalize case.
	pathCString := C.CString(path)
	defer C.free(unsafe.Pointer(pathCString))
	resultCString, errno := C.realpath(pathCString, nil)
	if resultCString == nil {
		return "", errno
	}
	defer C.free(unsafe.Pointer(resultCString))

	return C.GoString(resultCString), nil
}
