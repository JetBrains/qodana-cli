//go:build windows

/*
 * Copyright 2021-2024 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package utils

import (
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"strconv"
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

func RequestTermination(proc *os.Process) error {
	command := exec.Command("taskkill.exe", "/pid", strconv.Itoa(proc.Pid))
	output, err := command.CombinedOutput()
	log.Debugf("%s: %s", command.String(), output)

	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 1 {
			// Exit code 1 could mean at least:
			// - access denied
			// - the process can only be terminated forcefully
			// if the process can only be terminated forcefully, this is considered a "we tried" scenario
			// and no error should be returned. other errors with this exit code are logged and ignored.
			return nil
		}
		if exitErr.ExitCode() == 128 {
			// process not found (supposedly already exited)
			return os.ErrProcessDone
		}
	}

	return err
}
