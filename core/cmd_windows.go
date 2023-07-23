//go:build windows
// +build windows

package core

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// prepareWinCmd fixes cmd, so it can be run with a space in the batch or args path,
// fix taken from here https://github.com/golang/go/issues/17149
func prepareWinCmd(args ...string) *exec.Cmd {
	var commandLine = strings.Join(args, " ")
	var comSpec = os.Getenv("COMSPEC")
	if comSpec == "" {
		comSpec = os.Getenv("SystemRoot") + "\\System32\\cmd.exe"
	}
	var cmd = exec.Command(comSpec)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "/C \"" + commandLine + "\""}
	return cmd
}
