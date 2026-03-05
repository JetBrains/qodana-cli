//go:build !windows

package exec

import (
	"os"
	"syscall"
)

func RequestTermination(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}
