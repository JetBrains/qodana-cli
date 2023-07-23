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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
	log "github.com/sirupsen/logrus"
)

// lower a shortcut to strings.ToLower.
func lower(s string) string {
	return strings.ToLower(s)
}

// contains checks if a string is in a given slice.
func contains(s []string, str string) bool {
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
	if !contains(slice, elems[0]) {
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
		if contains(extensions, fileExtension) {
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

// findProcess using gopsutil to find process by name.
func findProcess(processName string) bool {
	if isDocker() {
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

// quoteIfSpace wraps in '"' if '`s`' contains space.
func quoteIfSpace(s string) string {
	if strings.Contains(s, " ") {
		return "\"" + s + "\""
	} else {
		return s
	}
}

// quoteForWindows wraps in '"' if '`s`' contains space on windows.
func quoteForWindows(s string) string {
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

// isDocker checks if Qodana is running in a Docker container.
func isDocker() bool {
	return os.Getenv(qodanaDockerEnv) != ""
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
