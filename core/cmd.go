/*
 * Copyright 2021-2023 JetBrains s.r.o.
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

package core

import (
	"errors"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
)

// RunCmd executes subprocess with forwarding of signals, and returns its exit code.
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
func RunCmd(cwd string, args ...string) int {
	log.Debugf("Running command: %v", args)
	cmd := exec.Command(args[0], args[1:]...)
	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "windows" {
		cmd = prepareWinCmd(args...)
	}
	if cwd != "" {
		cmd.Dir = cwd
	} else {
		wd, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		cmd.Dir = wd
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan)
	defer func() {
		signal.Reset()
		close(sigChan)
	}()

	for {
		select {
		case sig := <-sigChan:
			if err := cmd.Process.Signal(sig); err != nil && err.Error() != "os: process already finished" {
				log.Print("error sending signal", sig, err)
			}
		case err := <-waitCh:
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				waitStatus := exitError.Sys().(syscall.WaitStatus)
				if waitStatus.Exited() {
					return waitStatus.ExitStatus()
				}
				log.Println("Process killed (OOM?)")
				return QodanaOutOfMemoryExitCode
			}
			if err != nil {
				log.Println(err)
				return 1
			}
			return 0
		}
	}
}
