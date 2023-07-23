//go:build !windows
// +build !windows

package core

import (
	"log"
	"os/exec"
)

func prepareWinCmd(args ...string) *exec.Cmd {
	log.Fatal("Function should not be called on non-windows platforms")
	return nil
}
