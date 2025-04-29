//go:build unix

package utils

// #include <stdlib.h>
import "C"
import (
	"unsafe"
)

func Canonical(path string) (result string, err error) {
	pathCString := C.CString(path)
	defer C.free(unsafe.Pointer(pathCString))
	resultCString, errno := C.realpath(pathCString, nil)
	if resultCString == nil {
		return "", errno
	}
	defer C.free(unsafe.Pointer(resultCString))

	return C.GoString(resultCString), nil
}
