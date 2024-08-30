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
	"github.com/go-git/go-git/v5"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
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
	return gitRun(cwd, []string{"reset", "--soft", sha})
}

// GitResetBack aborts the git reset.
func GitResetBack(cwd string) error {
	return gitRun(cwd, []string{"reset", "'HEAD@{1}'"})
}

// GitCheckout checks out the given commit / branch.
func GitCheckout(cwd string, where string, force bool) error {
	if !force {
		return gitRun(cwd, []string{"checkout", where})
	} else {
		return gitRun(cwd, []string{"checkout", "-f", where})
	}
}

// GitClean cleans the git repository.
func GitClean(cwd string) error {
	return gitRun(cwd, []string{"clean", "-fdx"})
}

// GitRevisions returns the list of commits of the git repository in chronological order.
func GitRevisions(cwd string) []string {
	return reverse(GitLog(cwd, "%H", 0))
}

// GitRemoteUrl returns the remote url of the git repository.
func GitRemoteUrl(cwd string) (string, error) {
	repo, err := openRepository(cwd)
	if err != nil {
		return "", err
	}
	remote, err := repo.Remote("origin")
	if err != nil {
		return "", fmt.Errorf("failed to get origin remote: %s", err)
	}

	urls := remote.Config().URLs
	if len(urls) > 0 {
		return urls[0], nil
	} else {
		return "", fmt.Errorf("no URLs found for remote 'origin': %d", len(urls))
	}
}

// GitBranch returns the current branch of the git repository.
func GitBranch(cwd string) (string, error) {
	repo, err := openRepository(cwd)
	if err != nil {
		return "", err
	}
	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD reference: %v", err)
	}

	return ref.Name().Short(), nil
}

func GitCurrentRevision(cwd string) (string, error) {
	repo, err := openRepository(cwd)
	if err != nil {
		return "", err
	}
	ref, err := repo.Head()
	if err != nil {
		log.Fatalf("Failed to get HEAD reference: %v", err)
	}

	// Get the hash of the HEAD reference
	hash := ref.Hash()
	return hash.String(), nil
}

// openRepository finds the repository root directory and opens the Git repository
func openRepository(path string) (*git.Repository, error) {
	// Attempt to open the repository, starting from the given path and searching upwards
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %v", err)
	}
	return repo, nil
}
