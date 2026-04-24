package exec

import (
	"bytes"
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

// internalErrorExitCode is a sentinel value for errors that originate in this package
// (e.g., failed to start process). math.MinInt avoids colliding with real exit codes.
const internalErrorExitCode = math.MinInt

// OomExitCode is the conventional exit code for processes killed by OOM killer.
const OomExitCode = 137

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
	return execWithEnv(cwd, nil, stdout, stderr, timeout, timeoutExitCode, arg0, argv...)
}

func execWithEnv(
	cwd string,
	env []string,
	stdout io.Writer,
	stderr io.Writer,
	timeout time.Duration,
	timeoutExitCode int,
	arg0 string,
	argv ...string,
) (int, error) {
	if cwd == "" {
		return internalErrorExitCode, fmt.Errorf("cwd must not be empty: %w", os.ErrInvalid)
	}
	log.Debugf("Running command: %s %v", arg0, argv)
	cmd := exec.Command(arg0, argv...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Dir = cwd
	if env != nil {
		cmd.Env = env
	}
	if err := cmd.Start(); err != nil {
		return internalErrorExitCode, fmt.Errorf("failed to start command: %w", err)
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
	var stdout, stderr bytes.Buffer
	res, err := ExecWithTimeout(cwd, &stdout, &stderr, time.Duration(math.MaxInt64), 1, arg0, argv...)
	return stdout.String(), stderr.String(), res, err
}

// ExecRedirectOutputWithEnv is like ExecRedirectOutput but allows setting the
// subprocess environment. Pass os.Environ() plus any extra variables.
func ExecRedirectOutputWithEnv(cwd string, env []string, arg0 string, argv ...string) (string, string, int, error) {
	var stdout, stderr bytes.Buffer
	res, err := execWithEnv(cwd, env, &stdout, &stderr, time.Duration(math.MaxInt64), 1, arg0, argv...)
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
		signal.Stop(sigChan)
		close(sigChan)
	}()

	var timeoutCh = time.After(timeout)

	for {
		select {
		case <-sigChan:
			if err := RequestTermination(cmd.Process); err != nil && !errors.Is(
				err,
				os.ErrProcessDone,
			) {
				log.Error("Error terminating process: ", err)
			}
		case <-timeoutCh:
			if err := RequestTermination(cmd.Process); err != nil {
				log.Error("failed to kill process on timeout: ", err)
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
				return OomExitCode, nil
			}
			if ret != nil {
				log.Println(ret)
			}
			return cmd.ProcessState.ExitCode(), ret
		}
	}
}
