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
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/JetBrains/qodana-cli/internal/platform/strutil"
	"github.com/pterm/pterm"
	"github.com/shirou/gopsutil/v3/process"

	log "github.com/sirupsen/logrus"
)

// CheckDirFiles checks if a directory contains files.
func CheckDirFiles(dir string) bool {
	files, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(files) > 0
}

// FindFiles returns a slice of files with the given extensions from the given root (recursive).
func FindFiles(root string, extensions []string) []string {
	var files []string
	err := filepath.Walk(
		root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			fileExtension := filepath.Ext(path)
			if strutil.Contains(extensions, fileExtension) {
				files = append(files, path)
			}

			return nil
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	return files
}

func GetJavaExecutablePath() (string, error) {
	// java outputs settings to stderr, not stdout
	_, stderr, ret, err := ExecRedirectOutput(".", "java", "-XshowSettings:properties", "-version")
	if err != nil || ret != 0 {
		return "", fmt.Errorf(
			"failed to get JAVA_HOME: %w, %d. Check that java executable is accessible from the PATH",
			err,
			ret,
		)
	}

	// Parse stderr to find java.home line
	var javaHome string
	for _, line := range strutil.GetLines(stderr) {
		if strings.Contains(line, "java.home") {
			split := strings.SplitN(line, "=", 2)
			if len(split) >= 2 {
				javaHome = strings.TrimSpace(split[1])
				break
			}
		}
	}

	if javaHome == "" {
		return "", fmt.Errorf(
			"error while getting JAVA_HOME: java -XshowSettings:properties -version did not report java.home\n"+
				"  stderr: %s",
			stderr,
		)
	}

	javaExecutablePath := filepath.Join(javaHome, "bin", "java")
	if runtime.GOOS == "windows" {
		javaExecutablePath += ".exe"
	}
	return javaExecutablePath, nil
}

// LaunchAndLog launches a process and logs its output.
func LaunchAndLog(logDir string, executable string, args ...string) (string, string, int, error) {
	stdout, stderr, ret, err := ExecRedirectOutput(".", args[0], args[1:]...)
	if err != nil {
		log.Error(fmt.Errorf("failed to run %s: %w", executable, err))
		return "", "", ret, err
	}
	fmt.Println(stdout)
	if stderr != "" {
		_, _ = fmt.Fprintln(os.Stderr, stderr)
	}
	if err := AppendToFile(filepath.Join(logDir, executable+"-out.log"), stdout); err != nil {
		log.Error(err)
	}
	if err := AppendToFile(filepath.Join(logDir, executable+"-err.log"), stderr); err != nil {
		log.Error(err)
	}
	return stdout, stderr, ret, nil
}

// DownloadFile downloads a file from a given URL to a given filepath.
func DownloadFile(filepath string, url string, auth string, spinner *pterm.SpinnerPrinter) error {
	headReq, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return fmt.Errorf("error creating HEAD request: %w", err)
	}
	if auth != "" {
		headReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", auth))
	}
	response, err := http.DefaultClient.Do(headReq)
	if err != nil {
		return fmt.Errorf("error making HEAD request: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("response from %s (HEAD): %s", url, response.Status)
	}

	sizeStr := response.Header.Get("Content-Length")
	if sizeStr == "" {
		sizeStr = "-1"
	}
	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		return fmt.Errorf("error converting Content-Length to integer: %w", err)
	}

	getReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("error creating GET request: %w", err)
	}
	if auth != "" {
		getReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", auth))
	}
	resp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		return fmt.Errorf("error making GET request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("response from %s (GET): %s", url, resp.Status)
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			fmt.Printf("Error while closing HTTP stream: %v\n", err)
		}
	}(resp.Body)

	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	defer func(out *os.File) {
		if err := out.Close(); err != nil {
			fmt.Printf("Error while closing output file: %v\n", err)
		}
	}(out)

	buffer := make([]byte, 1024)
	total := 0
	lastTotal := 0
	text := ""
	if spinner != nil {
		text = spinner.Text
	}
	for {
		length, err := resp.Body.Read(buffer)
		if err != nil && err != io.EOF {
			return fmt.Errorf("error reading response body: %w", err)
		}
		total += length
		if spinner != nil && total-lastTotal > 1024*1024 {
			lastTotal = total
			spinner.UpdateText(fmt.Sprintf("%s (%d %%)", text, 100*total/size))
		}
		if length == 0 {
			break
		}
		if _, err = out.Write(buffer[:length]); err != nil {
			return fmt.Errorf("error writing to file: %w", err)
		}
	}

	// Check if the size matches, but only if the Content-Length header was present and valid
	if size > 0 && total != size {
		return fmt.Errorf("downloaded file size doesn't match expected size, got %d, expected %d", total, size)
	}

	if spinner != nil {
		spinner.UpdateText(fmt.Sprintf("%s (100 %%)", text))
	}

	return nil
}

func GetDefaultUser() string {
	switch runtime.GOOS {
	case "windows":
		return "root"
	default: // "darwin", "linux", "freebsd", "openbsd", "netbsd"
		return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	}
}

// FindProcess using gopsutil to find process by name.
func FindProcess(processName string) bool {
	if qdenv.IsContainer() {
		return IsProcess(processName)
	}
	p, err := process.Processes()
	if err != nil {
		log.Fatal(err)
	}
	for _, proc := range p {
		name, err := proc.Name()
		if err == nil {
			if name == processName {
				return true
			}
		}
	}
	return false
}

// IsProcess returns true if a process with cmd containing 'find' substring exists.
func IsProcess(find string) bool {
	processes, err := process.Processes()
	if err != nil {
		return false
	}
	for _, proc := range processes {
		cmd, err := proc.Cmdline()
		if err != nil {
			continue
		}
		if strings.Contains(cmd, find) {
			return true
		}
	}
	return false
}

// IsInstalled checks if git is installed.
func IsInstalled(what string) bool {
	help := ""
	if what == "git" {
		help = ", refer to https://git-scm.com/downloads for installing it"
	}

	_, err := exec.LookPath(what)
	if err != nil {
		msg.WarningMessage(
			"Unable to find %s"+help,
			what,
		)
		return false
	}
	return true
}

func OpenBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}
