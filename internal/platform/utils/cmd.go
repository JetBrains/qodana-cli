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
	bt "bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	// QodanaSuccessExitCode is Qodana exit code when the analysis is successfully completed.
	QodanaSuccessExitCode = 0
	// QodanaFailThresholdExitCode same as QodanaSuccessExitCode, but the threshold is set and exceeded.
	QodanaFailThresholdExitCode = 255
	// QodanaOutOfMemoryExitCode reports an interrupted process, sometimes because of an OOM.
	QodanaOutOfMemoryExitCode = 137
	// QodanaEapLicenseExpiredExitCode reports an expired license.
	QodanaEapLicenseExpiredExitCode = 7
	// QodanaTimeoutExitCodePlaceholder is not a real exit code (it is not obtained from IDE process! and not returned from CLI)
	// Placeholder used to identify the case when the analysis reached timeout
	QodanaTimeoutExitCodePlaceholder = 1000
	// QodanaEmptyChangesetExitCodePlaceholder is not a real exit code (it is not obtained from IDE process! and not returned from CLI)
	// Placeholder used to identify the case when the changeset for scoped analysis is empty
	QodanaEmptyChangesetExitCodePlaceholder = 2000
)

// Bootstrap takes the given command (from CLI or qodana.yaml) and runs it.
func Bootstrap(command string, project string) {
	if command == "" {
		return
	}
	var executor string
	var flag string
	switch runtime.GOOS {
	case "windows":
		executor = "cmd"
		flag = "/c"
	default:
		executor = "sh"
		flag = "-c"
	}

	if res, err := RunCmd(project, executor, flag, "\""+command+"\""); res > 0 || err != nil {
		log.Printf("Provided bootstrap command finished with error: %d. Exiting...", res)
		os.Exit(res)
	}
}

// RunCmd executes subprocess with forwarding of signals, and returns its exit code.
func RunCmd(cwd string, args ...string) (int, error) {
	return RunCmdWithTimeout(cwd, os.Stdout, os.Stderr, time.Duration(math.MaxInt64), 1, args...)
}

// RunCmdWithTimeout executes subprocess with forwarding of signals, and returns its exit code.
func RunCmdWithTimeout(
	cwd string,
	stdout io.Writer,
	stderr io.Writer,
	timeout time.Duration,
	timeoutExitCode int,
	args ...string,
) (int, error) {
	log.Debugf("Running command: %v", args)
	cmd := exec.Command("bash", "-c", strings.Join(args, " ")) // TODO : Viktor told about set -e
	var stdoutPipe, stderrPipe io.ReadCloser
	var err error
	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "windows" {
		cmd = prepareWinCmd(args...)
		stdoutPipe, err = cmd.StdoutPipe()
		if err != nil {
			return 1, fmt.Errorf("failed to get stdout pipe: %w", err)
		}
		stderrPipe, err = cmd.StderrPipe()
		if err != nil {
			return 1, fmt.Errorf("failed to get stderr pipe: %w", err)
		}
	} else {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
	}
	if cmd.Dir, err = getCwdPath(cwd); err != nil {
		return 1, err
	}
	cmd.Stdin = bt.NewBuffer([]byte{})
	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("failed to start command: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()

	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "windows" {
		go readAndWrite(stdoutPipe, stdout)
		go readAndWrite(stderrPipe, stderr)
	}
	return handleSignals(cmd, waitCh, timeout, timeoutExitCode)
}

// RunCmdRedirectOutput executes subprocess with forwarding of signals, returns stdout, stderr and exit code.
func RunCmdRedirectOutput(cwd string, args ...string) (string, string, int, error) {
	var stdout, stderr bt.Buffer
	res, err := RunCmdWithTimeout(cwd, &stdout, &stderr, time.Duration(math.MaxInt64), 1, args...)
	return stdout.String(), stderr.String(), res, err
}

// getCwdPath gets the current working directory path
func getCwdPath(cwd string) (string, error) {
	if cwd != "" {
		return cwd, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}
	return wd, nil
}

// handleSignals handles the signals from the subprocess
func handleSignals(cmd *exec.Cmd, waitCh <-chan error, timeout time.Duration, timeoutExitCode int) (int, error) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigChan) // Use Stop to prevent panics
		close(sigChan)
	}()

	var timeoutCh = time.After(timeout)

	for {
		select {
		case <-sigChan:
			if err := RequestTermination(cmd.Process); err != nil && !errors.Is(
				err,
				os.ErrProcessDone,
			) { // Use errors.Is for semantic comparison
				log.Error("Error terminating process: ", err)
			}
		case <-timeoutCh:
			if err := RequestTermination(cmd.Process); err != nil {
				log.Fatal("failed to kill process on timeout: ", err)
			}
			_, _ = cmd.Process.Wait()
			return timeoutExitCode, nil
		case ret := <-waitCh:
			var exitError *exec.ExitError
			if errors.As(ret, &exitError) {
				log.Println(ret)
				waitStatus := exitError.Sys().(syscall.WaitStatus)
				if waitStatus.Exited() {
					return waitStatus.ExitStatus(), nil
				}
				log.Println("Process killed (OOM?)")
				return QodanaOutOfMemoryExitCode, nil
			}
			if ret != nil {
				log.Println(ret)
			}
			return cmd.ProcessState.ExitCode(), ret
		}
	}
}

func readAndWrite(pipe io.ReadCloser, output io.Writer) {
	buf := make([]byte, 1024)
	for {
		n, err := pipe.Read(buf)
		if n > 0 {
			_, writeErr := output.Write(buf[:n])
			if writeErr != nil {
				log.Printf("failed to write to output: %v", writeErr)
				break
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("error reading from pipe: %v", err)
			}
			break
		}
	}
}
