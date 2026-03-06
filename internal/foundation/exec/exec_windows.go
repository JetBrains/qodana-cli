//go:build windows

package exec

import (
	"os"
	"os/exec"
	"strconv"

	log "github.com/sirupsen/logrus"
)

func RequestTermination(proc *os.Process) error {
	command := exec.Command("taskkill.exe", "/pid", strconv.Itoa(proc.Pid))
	output, err := command.CombinedOutput()
	log.Debugf("%s: %s", command.String(), output)

	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 1 {
			return nil
		}
		if exitErr.ExitCode() == 128 {
			return os.ErrProcessDone
		}
	}

	return err
}
