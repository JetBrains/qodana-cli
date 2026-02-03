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

package git

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/platform/algorithm"
	"github.com/JetBrains/qodana-cli/internal/platform/strutil"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	log "github.com/sirupsen/logrus"
)

// GitError is returned by gitRun when git has returned a non-zero exit code.
type GitError struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *GitError) CommandLine() string {
	return strings.Join(e.Args, " ")
}

func (e *GitError) Error() string {
	return fmt.Sprintf("%s exited with code %d\n  stderr: %s", e.CommandLine(), e.ExitCode, e.Stderr)
}

// gitRun runs the git command in the given directory and returns an error if any.
func gitRun(cwd string, command []string, logdir string) (string, string, error) {
	args := []string{"git"}
	args = append(args, command...)
	logger, err := LOGGER.GetLogger(logdir, "git")
	if err != nil {
		log.Errorf("Failed to create git logger: %v", err)
		return "", "", err
	}
	stdout, stderr, exitCode, err := utils.ExecRedirectOutput(cwd, args...)
	if logger != nil {
		logger.Printf("Executing command: %v", args)
		logger.Println(stdout)
	}
	if stderr != "" {
		if logger != nil {
			logger.Error(stderr + "\n")
		} else {
			log.Error(stderr)
		}
	}
	if exitCode != 0 {
		err := &GitError{
			Args:     args,
			ExitCode: exitCode,
			Stderr:   stderr,
		}
		log.Errorf("%s", err)
		return stdout, stderr, err
	}
	if err != nil {
		log.Errorf("An internal error occured while executing %s: %s", strings.Join(args, " "), err)
		return stdout, stderr, err
	}
	return stdout, stderr, nil
}

// RevParse converts any commit reference (hash, branch name, tag name, etc.) to a full SHA1 commit hash.
func RevParse(cwd string, ref string, logdir string) (string, error) {
	stdout, _, err := gitRun(cwd, []string{"rev-parse", "--verify", "--quiet", "--end-of-options", ref}, logdir)
	return strings.TrimSpace(stdout), err
}

// Reset resets the git repository to the given commit.
func Reset(cwd string, sha string, logdir string) error {
	_, _, err := gitRun(cwd, []string{"reset", "--soft", sha}, logdir)
	return err
}

// ResetBack aborts the git reset.
func ResetBack(cwd string, logdir string) error {
	_, _, err := gitRun(cwd, []string{"reset", "HEAD@{1}"}, logdir)
	return err
}

// CheckoutAndUpdateSubmodule performs a git checkout to the specified rev and updates submodules recursively, QD-10767.
func CheckoutAndUpdateSubmodule(cwd string, ref string, force bool, logdir string) error {
	// Checkout the root repository
	if _, err := RevParse(cwd, ref, logdir); err != nil {
		// It would be impossible to checkout this commit if it's not rev-parseable.
		errMessage := fmt.Sprintf("cannot checkout: cannot resolve reference '%s'", ref)

		if isShallowClone(cwd, logdir) {
			errMessage += "\n  Hint: you appear to be working in a shallow clone. Consider running 'git fetch --unshallow' or checking out without '--depth'."
		}

		return errors.New(errMessage)
	}

	if err := checkout(cwd, ref, force, logdir); err != nil {
		return err
	}

	if err := updateSubmodules(cwd, force, logdir); err != nil {
		return err
	}

	return nil
}

// updateSubmodules performs a recursive submodule update to the ref specified in git cache.
func updateSubmodules(root string, force bool, logdir string) error {
	// Note: git submodule update is not used because it fails on corrupted repositories where .gitmodules data does not
	// match git cache. This can often happen when a user removes a submodule incorrectly.
	submodules, err := getSubmodules(root, logdir)
	if err != nil {
		return err
	}

	for _, path := range submodules {
		fullPath := filepath.Join(root, path)

		updateArgs := []string{"submodule", "update", "--init"}
		if force {
			updateArgs = append(updateArgs, "--force")
		}
		updateArgs = append(updateArgs, "--", path)

		_, _, err := gitRun(root, updateArgs, logdir)
		if err != nil {
			return fmt.Errorf("failed to update submodule %q: %w", path, err)
		}

		// Recurse down
		err = updateSubmodules(fullPath, force, logdir)
		if err != nil {
			return fmt.Errorf("failed to update submodules of %q: %w", fullPath, err)
		}
	}

	return nil
}

func getDeclaredSubmodules(cwd string, logdir string) ([]string, error) {
	_, err := os.Stat(filepath.Join(cwd, ".gitmodules"))
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}

	stdout, _, err := gitRun(cwd, []string{"config", "get", "--file", ".gitmodules", "--regexp", "--all", ".path$"}, logdir)
	if err != nil {
		var gitErr *GitError
		if errors.As(err, &gitErr) && gitErr.ExitCode == 1 {
			// no items found - .gitmodules may be empty
			return []string{}, nil
		}
		return nil, err
	}
	return strutil.GetLines(stdout), nil
}

// getSubmodules returns all submodules of a repository (non-recursive). Only submodules that are present in BOTH
// .gitmodules and git cache are reported.
func getSubmodules(cwd string, logdir string) ([]string, error) {
	declaredSubmodules, err := getDeclaredSubmodules(cwd, logdir)
	if err != nil {
		return nil, err
	}
	return algorithm.Filter(declaredSubmodules, func(path string) bool {
		mode, err := getObjectMode(cwd, path, logdir)
		if err != nil {
			log.Debugf("Ignoring declared submodule %q: ls-files failed: %v", path, err)
			return false
		}
		if mode != 160000 {
			log.Debugf("Ignoring declared submodule %q: git cache reports mode %d (expected 160000)", path, mode)
			return false
		}

		return true
	}), nil
}

func getObjectMode(cwd string, path string, logdir string) (int, error) {
	stdout, _, err := gitRun(cwd, []string{"ls-files", "--format=%(objectmode)", "--", path}, logdir)
	if err != nil {
		return 0, err
	}
	stdout = strings.TrimSpace(stdout)

	if stdout == "" {
		return 0, nil
	}

	mode := 0
	if _, err = fmt.Sscanf(stdout, "%d", &mode); err != nil {
		return 0, err
	}

	return mode, nil
}

// isShallowClone checks if the repository is a shallow clone.
func isShallowClone(cwd string, logdir string) bool {
	stdout, _, err := gitRun(cwd, []string{"rev-parse", "--is-shallow-repository"}, logdir)
	return err == nil && strings.TrimSpace(stdout) == "true"
}

// checkout checks out the given commit / branch.
func checkout(cwd string, where string, force bool, logdir string) error {
	var err error
	if !force {
		_, _, err = gitRun(cwd, []string{"checkout", where}, logdir)
	} else {
		_, _, err = gitRun(cwd, []string{"checkout", "-f", where}, logdir)
	}
	return err
}

// GitSubmoduleUpdate updates submodules according to current revision
func submoduleUpdate(cwd string, force bool, logdir string) error {
	if !force {
		_, _, err := gitRun(cwd, []string{"submodule", "update", "--init", "--recursive"}, logdir)
		return err
	}
	_, _, err := gitRun(cwd, []string{"submodule", "update", "--init", "--recursive", "--force"}, logdir)
	return err
}

// Clean cleans the git repository.
func Clean(cwd string, logdir string) error {
	_, _, err := gitRun(cwd, []string{"clean", "-fdx"}, logdir)
	return err
}

// Revisions returns the list of commits of the git repository in chronological order.
func Revisions(cwd string) []string {
	return strutil.Reverse(Log(cwd, "%H", 0))
}

// Root returns absolute path of repo root
func Root(cwd string, logdir string) (string, error) {
	stdout, _, err := gitRun(cwd, []string{"rev-parse", "--show-toplevel"}, logdir)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}

// RemoteUrl returns the remote url of the git repository.
func RemoteUrl(cwd string, logdir string) (string, error) {
	stdout, _, err := gitRun(cwd, []string{"remote", "get-url", "origin"}, logdir)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}

// Branch returns the current branch of the git repository.
func Branch(cwd string, logdir string) (string, error) {
	stdout, _, err := gitRun(cwd, []string{"rev-parse", "--abbrev-ref", "HEAD"}, logdir)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}

func CurrentRevision(cwd string, logdir string) (string, error) {
	stdout, _, err := gitRun(cwd, []string{"rev-parse", "HEAD"}, logdir)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}

// RevisionExists returns true when revision exists in history.
func RevisionExists(cwd string, revision string, logdir string) bool {
	_, stderr, err := gitRun(cwd, []string{"show", "--no-patch", revision}, logdir)
	if strings.Contains(stderr, revision) || strings.Contains(stderr, "fatal:") || err != nil {
		return false
	}
	return true
}
