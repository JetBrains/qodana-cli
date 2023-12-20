//go:build !windows

package platform

import (
	"log"
	"os/exec"
)

//goland:noinspection GoUnusedParameter
func prepareWinCmd(args ...string) *exec.Cmd {
	log.Fatal("Function should not be called on non-windows platforms")
	return nil
}
