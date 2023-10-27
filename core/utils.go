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
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/pterm/pterm"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
	log "github.com/sirupsen/logrus"
)

// lower a shortcut to strings.ToLower.
func lower(s string) string {
	return strings.ToLower(s)
}

// Contains checks if a string is in a given slice.
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
func Contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// reverse reverses the given string slice.
func reverse(s []string) []string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// getHash returns a SHA256 hash of a given string.
func getHash(s string) string {
	sha256sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sha256sum[:])
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

// CheckDirFiles checks if a directory contains files.
func CheckDirFiles(dir string) bool {
	files, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(files) > 0
}

// findFiles returns a slice of files with the given extensions from the given root (recursive).
func findFiles(root string, extensions []string) []string {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		fileExtension := filepath.Ext(path)
		if Contains(extensions, fileExtension) {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	return files
}

// getAzureJobUrl returns the Azure Pipelines job URL.
func getAzureJobUrl() string {
	if server := os.Getenv("SYSTEM_TEAMFOUNDATIONCOLLECTIONURI"); server != "" {
		return strings.Join([]string{
			server,
			os.Getenv("SYSTEM_TEAMPROJECT"),
			"/_build/results?buildId=",
			os.Getenv("BUILD_BUILDID"),
		}, "")
	}
	return ""
}

// getSpaceJobUrl returns the Space job URL.
func getSpaceRemoteUrl() string {
	if server := os.Getenv("JB_SPACE_API_URL"); server != "" {
		return strings.Join([]string{
			"ssh://git@git.",
			server,
			"/",
			os.Getenv("JB_SPACE_PROJECT_KEY"),
			"/",
			os.Getenv("JB_SPACE_GIT_REPOSITORY_NAME"),
			".git",
		}, "")
	}
	return ""
}

// findProcess using gopsutil to find process by name.
func findProcess(processName string) bool {
	if IsContainer() {
		return isProcess(processName)
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

// isProcess returns true if a process with cmd containing 'find' substring exists.
func isProcess(find string) bool {
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

// QuoteIfSpace wraps in '"' if '`s`' Contains space.
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
func QuoteIfSpace(s string) string {
	if strings.Contains(s, " ") {
		return "\"" + s + "\""
	} else {
		return s
	}
}

// QuoteForWindows wraps in '"' if '`s`' contains space on windows.
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
func QuoteForWindows(s string) string {
	if //goland:noinspection GoBoolExpressions
	strings.Contains(s, " ") && runtime.GOOS == "windows" {
		return "\"" + s + "\""
	} else {
		return s
	}
}

// getRemoteUrl returns remote url of the current git repository.
func getRemoteUrl() string {
	url := os.Getenv(qodanaRemoteUrl)
	if url == "" {
		out, err := exec.Command("git", "remote", "get-url", "origin").Output()
		if err != nil {
			return ""
		}
		url = string(out)
	}
	return strings.TrimSpace(url)
}

// getDeviceIdSalt set consistent device.id based on given repo upstream #SA-391.
func getDeviceIdSalt() []string {
	salt := os.Getenv("SALT")
	deviceId := os.Getenv("DEVICEID")
	if salt == "" || deviceId == "" {
		hash := "00000000000000000000000000000000"
		remoteUrl := getRemoteUrl()
		if remoteUrl != "" {
			hash = fmt.Sprintf("%x", md5.Sum(append([]byte("1n1T-$@Lt-"), remoteUrl...)))
		}
		if salt == "" {
			salt = fmt.Sprintf("%x", md5.Sum([]byte("$eC0nd-$@Lt-"+hash)))
		}
		if deviceId == "" {
			deviceId = fmt.Sprintf("200820300000000-%s-%s-%s-%s", hash[0:4], hash[4:8], hash[8:12], hash[12:24])
		}
	}
	return []string{deviceId, salt}
}

func writeFileIfNew(filepath string, content string) {
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		if err := os.WriteFile(filepath, []byte(content), 0o755); err != nil {
			log.Fatal(err)
		}
	}
}

func getPluginIds(plugins []Plugin) []string {
	ids := make([]string, len(plugins))
	for i, plugin := range plugins {
		ids[i] = plugin.Id
	}
	return ids
}

// IsContainer checks if Qodana is running in a container.
func IsContainer() bool {
	return os.Getenv(qodanaDockerEnv) != ""
}

// isInstalled checks if git is installed.
func isInstalled(what string) bool {
	help := ""
	if what == "git" {
		help = ", refer to https://git-scm.com/downloads for installing it"
	}

	_, err := exec.LookPath(what)
	if err != nil {
		WarningMessage(
			"Unable to find %s"+help,
			what,
		)
		return false
	}
	return true
}

// createUser will make dynamic uid as a valid user `idea`, needed for gradle cache.
func createUser(fn string) {
	if //goland:noinspection ALL
	os.Getuid() == 0 {
		return
	}
	idea := fmt.Sprintf("idea:x:%d:%d:idea:/root:/bin/bash", os.Getuid(), os.Getgid())
	data, err := os.ReadFile(fn)
	if err != nil {
		log.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		if line == idea {
			return
		}
	}
	if err = os.WriteFile(fn, []byte(strings.Join(append(lines, idea), "\n")), 0o777); err != nil {
		log.Fatal(err)
	}
}

func downloadFile(filepath string, url string, spinner *pterm.SpinnerPrinter) error {
	response, err := http.Head(url)
	if err != nil {
		return err
	}
	size, _ := strconv.Atoi(response.Header.Get("Content-Length"))

	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatalf("Error while closing HTTP stream: %v", err)
		}
	}(resp.Body)

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}

	defer func(out *os.File) {
		err := out.Close()
		if err != nil {
			log.Fatalf("Error while closing output file: %v", err)
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
			return err
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
			return err
		}
	}

	if total != size {
		return fmt.Errorf("downloaded file size doesn't match expected size")
	}

	if spinner != nil {
		spinner.UpdateText(fmt.Sprintf("%s (100 %%)", text))
	}

	return nil
}
