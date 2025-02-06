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
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/pterm/pterm"
	"github.com/shirou/gopsutil/v3/process"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// Lower a shortcut to strings.ToLower.
func Lower(s string) string {
	return strings.ToLower(s)
}

// Contains checks if a string is in a given slice.
func Contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// Append appends a string to a slice if it's not already there.
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
func Append(slice []string, elems ...string) []string {
	if !Contains(slice, elems[0]) {
		slice = append(slice, elems[0])
	}
	return slice
}

func Remove(s []string, r string) []string {
	for i, v := range s {
		if v == r {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

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
			if Contains(extensions, fileExtension) {
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

// QuoteIfSpace wraps in '"' if '`s`' Contains space.
func QuoteIfSpace(s string) string {
	if strings.Contains(s, " ") {
		return "\"" + s + "\""
	} else {
		return s
	}
}

// QuoteForWindows wraps in '"' if '`s`' contains space on windows.
func QuoteForWindows(s string) string {
	if //goland:noinspection GoBoolExpressions
	strings.Contains(s, " ") && runtime.GOOS == "windows" {
		return "\"" + s + "\""
	} else {
		return s
	}
}

func GetJavaExecutablePath() (string, error) {
	var java string
	var err error
	var ret int
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS == "windows" {
		java, _, ret, err = RunCmdRedirectOutput("", "java -XshowSettings:properties -version 2>&1 | findstr java.home")
	} else {
		java, _, ret, err = RunCmdRedirectOutput("", "java -XshowSettings:properties -version 2>&1 | grep java.home")
	}
	if err != nil || ret != 0 {
		return "", fmt.Errorf(
			"failed to get JAVA_HOME: %w, %d. Check that java executable is accessible from the PATH",
			err,
			ret,
		)
	}
	split := strings.Split(java, "=")
	if len(split) < 2 {
		return "", fmt.Errorf(
			"failed to get JAVA_HOME: %s. Check that java executable is accessible from the PATH",
			java,
		)
	}

	javaHome := split[1]
	javaHome = strings.Trim(javaHome, "\r\n ")

	var javaExecFileName string
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS == "windows" {
		javaExecFileName = "java.exe"
	} else {
		javaExecFileName = "java"
	}

	javaExecutablePath := filepath.Join(javaHome, "bin", javaExecFileName)
	return javaExecutablePath, nil
}

// LaunchAndLog launches a process and logs its output.
func LaunchAndLog(logDir string, executable string, args ...string) (string, string, int, error) {
	stdout, stderr, ret, err := RunCmdRedirectOutput("", args...)
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
func DownloadFile(filepath string, url string, spinner *pterm.SpinnerPrinter) error {
	response, err := http.Head(url)
	if err != nil {
		return fmt.Errorf("error making HEAD request: %w", err)
	}

	sizeStr := response.Header.Get("Content-Length")
	if sizeStr == "" {
		sizeStr = "-1"
	}
	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		return fmt.Errorf("error converting Content-Length to integer: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("error making GET request: %w", err)
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

// Reverse reverses the given string slice.
func Reverse(s []string) []string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
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
