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
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func GitDiffNameOnly(cwd string, diffStart string, diffEnd string) ([]string, error) {
	repo, err := openRepository(cwd)
	if err != nil {
		return []string{""}, err
	}
	files, err := getChangedFilesBetweenCommits(repo, cwd, diffStart, diffEnd)
	if err != nil {
		return []string{""}, err
	}
	return files, nil
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

// getChangedFilesBetweenCommits retrieves changed files between two commit hashes
func getChangedFilesBetweenCommits(repo *git.Repository, cwd, hash1, hash2 string) ([]string, error) {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path of root folder %s: %v", cwd, err)
	}
	commit1, err := repo.CommitObject(plumbing.NewHash(hash1))
	if err != nil {
		return nil, fmt.Errorf("failed to find commit %s: %v", hash1, err)
	}

	commit2, err := repo.CommitObject(plumbing.NewHash(hash2))
	if err != nil {
		return nil, fmt.Errorf("failed to find commit %s: %v", hash2, err)
	}

	tree1, err := commit1.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get tree for commit %s: %v", hash1, err)
	}

	tree2, err := commit2.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get tree for commit %s: %v", hash2, err)
	}

	changes, err := object.DiffTree(tree1, tree2)
	if err != nil {
		return nil, fmt.Errorf("failed to get changes between commits %s and %s: %v", hash1, hash2, err)
	}

	changedFilesMap := make(map[string]struct{})

	for _, change := range changes {
		if change.From.Name != "" {
			changedFilesMap[change.From.Name] = struct{}{}
		}
		if change.To.Name != "" {
			changedFilesMap[change.To.Name] = struct{}{}
		}
	}

	var changedFiles = make([]string, 0)
	for file := range changedFilesMap {
		absolutePath, err := getAbsolutePath(repo, file)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path for file %s: %v", file, err)
		}
		isInSubfolder := strings.HasPrefix(absolutePath, absCwd+string(filepath.Separator))
		if isInSubfolder {
			changedFiles = append(changedFiles, absolutePath)
		}
	}

	return changedFiles, nil
}

// getAbsolutePath returns the absolute path of a file relative to the repository root
func getAbsolutePath(repo *git.Repository, relativePath string) (string, error) {
	// Get the repository's working directory
	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %v", err)
	}
	repoRoot := worktree.Filesystem.Root()

	// Combine the repository root with the relative path to get the absolute path
	absolutePath, err := filepath.Abs(filepath.Join(repoRoot, relativePath))

	return absolutePath, err
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
