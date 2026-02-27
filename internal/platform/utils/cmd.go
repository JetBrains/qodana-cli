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
	// QodanaInternalErrorExitCode is returned when the CLI itself fails (e.g. invalid arguments, failed to start process).
	// It is not a real process exit code. Use this to distinguish CLI errors from subprocess exit codes.
	QodanaInternalErrorExitCode = math.MinInt
)

// Bootstrap takes the given command (from CLI or qodana.yaml) and runs it.
func Bootstrap(command string, project string) {
	if command == "" {
		return
	}
	if res, err := RunShell(project, command); res > 0 || err != nil {
		log.Printf("Provided bootstrap command finished with error: %d. Exiting...", res)
		os.Exit(res)
	}
}

// Exec executes subprocess with forwarding of signals, and returns its exit code.
func Exec(cwd string, arg0 string, argv ...string) (int, error) {
	return ExecWithTimeout(cwd, os.Stdout, os.Stderr, time.Duration(math.MaxInt64), 1, arg0, argv...)
}

// ExecWithTimeout executes subprocess with forwarding of signals, and returns its exit code.
func ExecWithTimeout(
	cwd string,
	stdout io.Writer,
	stderr io.Writer,
	timeout time.Duration,
	timeoutExitCode int,
	arg0 string,
	argv ...string,
) (int, error) {
	if cwd == "" {
		return QodanaInternalErrorExitCode, fmt.Errorf("cwd must not be empty: %w", os.ErrInvalid)
	}
	log.Debugf("Running command: %s %v", arg0, argv)
	cmd := exec.Command(arg0, argv...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Dir = cwd
	cmd.Stdin = bt.NewBuffer([]byte{})
	if err := cmd.Start(); err != nil {
		return QodanaInternalErrorExitCode, fmt.Errorf("failed to start command: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()

	return handleSignals(cmd, waitCh, timeout, timeoutExitCode)
}

// ExecRedirectOutput executes subprocess with forwarding of signals, returns stdout, stderr and exit code.
func ExecRedirectOutput(cwd string, arg0 string, argv ...string) (string, string, int, error) {
	var stdout, stderr bt.Buffer
	res, err := ExecWithTimeout(cwd, &stdout, &stderr, time.Duration(math.MaxInt64), 1, arg0, argv...)
	return stdout.String(), stderr.String(), res, err
}

// RunShell executes a shell command (using cmd on Windows, sh on other platforms).
func RunShell(cwd string, command string) (int, error) {
	argv := getSystemShellArgv(command)
	return Exec(cwd, argv[0], argv[1:]...)
}

// RunShellRedirectOutput executes a shell command and captures stdout/stderr.
func RunShellRedirectOutput(cwd string, command string) (string, string, int, error) {
	argv := getSystemShellArgv(command)
	return ExecRedirectOutput(cwd, argv[0], argv[1:]...)
}

// getSystemShellArgv the arguments to invoke the system shell with the specified command.
func getSystemShellArgv(command string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", command}
	}
	return []string{"sh", "-c", command}
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
			<-waitCh
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
