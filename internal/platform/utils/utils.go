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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/foundation/download"
	fexec "github.com/JetBrains/qodana-cli/internal/foundation/exec"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	"github.com/JetBrains/qodana-cli/internal/foundation/str"
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/pterm/pterm"
	"github.com/shirou/gopsutil/v3/process"

	log "github.com/sirupsen/logrus"
)

// Bootstrap takes the given command (from CLI or qodana.yaml) and runs it.
func Bootstrap(command string, project string) {
	if command == "" {
		return
	}
	if res, err := fexec.RunShell(project, command); res > 0 || err != nil {
		log.Printf("Provided bootstrap command finished with error: %d. Exiting...", res)
		os.Exit(res)
	}
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
			if str.Contains(extensions, fileExtension) {
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

// LaunchAndLog launches a process and logs its output.
// The actual executable is args[0]; logLabel is only used for log file names.
func LaunchAndLog(logDir string, logLabel string, args []string) (string, string, int, error) {
	stdout, stderr, ret, err := fexec.ExecRedirectOutput(".", args[0], args[1:]...)
	if err != nil {
		log.Error(fmt.Errorf("failed to run %s: %w", logLabel, err))
		return "", "", ret, err
	}
	fmt.Println(stdout)
	if stderr != "" {
		_, _ = fmt.Fprintln(os.Stderr, stderr)
	}
	if err := fs.AppendToFile(filepath.Join(logDir, logLabel+"-out.log"), stdout); err != nil {
		log.Error(err)
	}
	if err := fs.AppendToFile(filepath.Join(logDir, logLabel+"-err.log"), stderr); err != nil {
		log.Error(err)
	}
	return stdout, stderr, ret, nil
}

// DownloadFile downloads a file from url to filepath, optionally authenticating with a Bearer token
// and reporting progress on a spinner. The write is atomic (temp file + rename, via foundation/download).
func DownloadFile(filepath string, url string, auth string, spinner *pterm.SpinnerPrinter) error {
	var progress func(downloaded, total int64)
	if spinner != nil {
		baseText := spinner.Text
		var lastReported int64
		progress = func(downloaded, total int64) {
			if total <= 0 {
				return
			}
			// Throttle to ~1 update per MiB; always show the final 100%.
			if downloaded != total && downloaded-lastReported < 1024*1024 {
				return
			}
			lastReported = downloaded
			spinner.UpdateText(fmt.Sprintf("%s (%d %%)", baseText, 100*downloaded/total))
		}
	}
	_, err := download.ToFile(url, filepath, download.Options{Bearer: auth, Progress: progress})
	return err
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
