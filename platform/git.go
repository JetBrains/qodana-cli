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

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
)

// gitRun runs the git command in the given directory and returns an error if any.
func gitRun(cwd string, command []string) error {
	cmd := exec.Command("git", command...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// GitReset resets the git repository to the given commit.
func GitReset(cwd string, sha string) error {
	return gitRun(cwd, []string{"reset", "--soft", strings.TrimPrefix(sha, "CI")})
}

// GitResetBack aborts the git reset.
func GitResetBack(cwd string) error {
	return gitRun(cwd, []string{"reset", "'HEAD@{1}'"})
}

// GitCheckout checks out the given commit / branch.
func GitCheckout(cwd string, where string) error {
	return gitRun(cwd, []string{"checkout", where})
}

// GitClean cleans the git repository.
func GitClean(cwd string) error {
	return gitRun(cwd, []string{"clean", "-fdx"})
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

// GitLog returns the git log of the given repository in the given format.
func GitLog(cwd string, format string, since int) []string {
	args := []string{"--no-pager", "log"}
	if format != "" {
		args = append(args, "--pretty=format:"+format)
	}
	if since > 0 {
		args = append(args, fmt.Sprintf("--since=%d.days", since))
	}
	return gitOutput(cwd, args)
}

// GitRevisions returns the list of commits of the git repository in chronological order.
func GitRevisions(cwd string) []string {
	return reverse(GitLog(cwd, "%H", 0))
}

// GitRemoteUrl returns the remote url of the git repository.
func GitRemoteUrl(cwd string) string {
	return gitOutput(cwd, []string{"remote", "get-url", "origin"})[0]
}

// GitBranch returns the current branch of the git repository.
func GitBranch(cwd string) string {
	return gitOutput(cwd, []string{"rev-parse", "--abbrev-ref", "HEAD"})[0]
}
