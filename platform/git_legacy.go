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

package platform

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os/exec"
	"path/filepath"
	"strings"
)

func GitBranchLegacy(cwd string) string {
	return gitOutput(cwd, []string{"rev-parse", "--abbrev-ref", "HEAD"})[0]
}

func GitDiffNameOnlyLegacy(cwd string, diffStart string, diffEnd string) []string {
	relPaths := gitOutput(cwd, []string{"diff", "--name-only", "--relative", diffStart, diffEnd})
	absPaths := make([]string, 0)
	for _, relPath := range relPaths {
		if relPath == "" {
			continue
		}
		filePath := filepath.Join(cwd, relPath)
		absFilePath, err := filepath.Abs(filePath)
		if err != nil {
			log.Fatalf("Failed to resolve absolute path of %s: %s", filePath, err)
		}
		absPaths = append(absPaths, absFilePath)
	}
	return absPaths
}

func GitCurrentRevisionLegacy(cwd string) string {
	return gitOutput(cwd, []string{"rev-parse", "HEAD"})[0]
}

func GitRemoteUrlLegacy(cwd string) string {
	return gitOutput(cwd, []string{"remote", "get-url", "origin"})[0]
}

// GitLog returns the git log of the given repository in the given format.
func GitLog(cwd string, format string, since int) []string {
	args := []string{"--no-pager", "log", "--all"}
	if format != "" {
		args = append(args, "--pretty=format:"+format)
	}
	if since > 0 {
		args = append(args, fmt.Sprintf("--since=%d.days", since))
	}
	return gitOutput(cwd, args)
}

// gitOutput runs the git command in the given directory and returns the output.
func gitOutput(cwd string, args []string) []string {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		log.Warn(err.Error())
		return []string{""}
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}