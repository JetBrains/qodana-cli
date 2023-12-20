package platform

import (
	bt "bytes"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
)

// RunCmd executes subprocess with forwarding of signals, and returns its exit code.
func RunCmd(cwd string, args ...string) (int, error) {
	return RunCmdWithForward(cwd, os.Stdout, os.Stderr, args...)
}

// RunCmdWithForward executes subprocess with forwarding of signals, and returns its exit code.
func RunCmdWithForward(cwd string, stdout *os.File, stderr *os.File, args ...string) (int, error) {
	log.Debugf("Running command: %v", args)
	cmd := exec.Command("bash", "-c", strings.Join(args, " ")) // TODO : Viktor told about set -e
	if                                                         //goland:noinspection GoBoolExpressions
	runtime.GOOS == "windows" {
		cmd = prepareWinCmd(args...)
	}
	var err error
	if cmd.Dir, err = getCwdPath(cwd); err != nil {
		return 1, err
	}
	cmd.Stdin = bt.NewBuffer([]byte{})
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("failed to start command: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()

	return handleSignals(cmd, waitCh)
}

// closePipe closes the pipe
func closePipe(file *os.File) {
	err := file.Close()
	if err != nil {
		log.Error(err)
	}
}

// RunCmdRedirectOutput executes subprocess with forwarding of signals, returns stdout, stderr and exit code.
func RunCmdRedirectOutput(cwd string, args ...string) (string, string, int, error) {
	outReader, outWriter, err := os.Pipe()
	if err != nil {
		return "", "", -1, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	defer closePipe(outReader)
	errReader, errWriter, err := os.Pipe()
	if err != nil {
		return "", "", -1, fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	defer closePipe(errReader)

	outChannel := make(chan string)
	errChannel := make(chan string)

	go copyToChannel(outReader, outChannel)
	go copyToChannel(errReader, errChannel)

	res, err := RunCmdWithForward(cwd, outWriter, errWriter, args...)
	closePipes(outWriter, errWriter)
	stdout := <-outChannel
	stderr := <-errChannel
	return stdout, stderr, res, err
}

// closePipes closes the pairs of pipes
func closePipes(outWriter *os.File, errWriter *os.File) {
	err := outWriter.Close()
	if err != nil {
		log.Error("Error while closing stdout: ", err)
	}
	err = errWriter.Close()
	if err != nil {
		log.Error("Error while closing stderr: ", err)
	}
}

// copyToChannel copies the content of a Reader to a channel
func copyToChannel(reader io.Reader, ch chan<- string) {
	var buf bt.Buffer
	_, err := io.Copy(&buf, reader)
	if err != nil {
		log.Error(err)
	}
	ch <- buf.String()
	close(ch)
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
func handleSignals(cmd *exec.Cmd, waitCh <-chan error) (int, error) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan)
	defer func() {
		signal.Stop(sigChan) // Use Stop to prevent panics
		close(sigChan)
	}()

	for {
		select {
		case sig := <-sigChan:
			if err := cmd.Process.Signal(sig); err != nil && !errors.Is(err, os.ErrProcessDone) { // Use errors.Is for semantic comparison
				log.Error("Error sending signal: ", sig, err)
			}
		case ret := <-waitCh:
			return getExitCode(ret), nil
		}
	}
}

// getExitCode gets the exit code of the subprocess
func getExitCode(err error) int {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		log.Println(err)
		waitStatus := exitError.Sys().(syscall.WaitStatus)
		if waitStatus.Exited() {
			return waitStatus.ExitStatus()
		}
		log.Println("Process killed (OOM?)")
		return 137 // QodanaOutOfMemoryExitCode
	}
	if err != nil {
		log.Println(err)
		return 1
	}
	return 0
}
