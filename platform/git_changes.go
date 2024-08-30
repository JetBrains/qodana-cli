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
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	"path/filepath"
	"regexp"
	"strings"
)

type ChangedRegion struct {
	FirstLine int `json:"firstLine"`
	Count     int `json:"count"`
}

type ChangedFile struct {
	Path    string           `json:"path"`
	Added   []*ChangedRegion `json:"added"`
	Deleted []*ChangedRegion `json:"deleted"`
}

type ChangedFiles struct {
	Files []*ChangedFile `json:"files"`
}

func GitDiffNameOnly(cwd string, diffStart string, diffEnd string) ([]string, error) {
	repo, err := openRepository(cwd)
	if err != nil {
		return []string{""}, err
	}
	changedFiles, err := getChangedFilesBetweenCommits(repo, cwd, diffStart, diffEnd)
	if err != nil {
		return []string{""}, err
	}

	var changedFileNames = make([]string, 0, len(changedFiles.Files))
	for _, file := range changedFiles.Files {
		changedFileNames = append(changedFileNames, file.Path)
	}

	return changedFileNames, nil
}

func GitChangedFiles(cwd string, diffStart string, diffEnd string) (ChangedFiles, error) {
	repo, err := openRepository(cwd)
	if err != nil {
		return ChangedFiles{}, err
	}
	return getChangedFilesBetweenCommits(repo, cwd, diffStart, diffEnd)
}

// getChangedFilesBetweenCommits retrieves changed files between two commit hashes
func getChangedFilesBetweenCommits(repo *git.Repository, cwd, hash1, hash2 string) (ChangedFiles, error) {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return ChangedFiles{}, fmt.Errorf("failed to get absolute path of root folder %s: %v", cwd, err)
	}
	commit1, err := repo.CommitObject(plumbing.NewHash(hash1))
	if err != nil {
		return ChangedFiles{}, fmt.Errorf("failed to find commit %s: %v", hash1, err)
	}

	commit2, err := repo.CommitObject(plumbing.NewHash(hash2))
	if err != nil {
		return ChangedFiles{}, fmt.Errorf("failed to find commit %s: %v", hash2, err)
	}

	tree1, err := commit1.Tree()
	if err != nil {
		return ChangedFiles{}, fmt.Errorf("failed to get tree for commit %s: %v", hash1, err)
	}

	tree2, err := commit2.Tree()
	if err != nil {
		return ChangedFiles{}, fmt.Errorf("failed to get tree for commit %s: %v", hash2, err)
	}

	changes, err := object.DiffTree(tree1, tree2)
	if err != nil {
		return ChangedFiles{}, fmt.Errorf("failed to get changes between commits %s and %s: %v", hash1, hash2, err)
	}

	changedFilesMap := make(map[string]*ChangedFile)
	repoRoot, err := getRepoRoot(repo, err)

	for _, change := range changes {
		var path = ""
		if change.From.Name != "" {
			path = change.From.Name
		} else {
			path = change.To.Name
		}
		if path == "" {
			continue
		}
		absolutePath, err := filepath.Abs(filepath.Join(repoRoot, path))
		if err != nil {
			return ChangedFiles{}, fmt.Errorf("failed to get absolute path for file %s: %v", path, err)
		}
		if !strings.HasPrefix(absolutePath, absCwd+string(filepath.Separator)) {
			continue
		}

		changedFile, exists := changedFilesMap[absolutePath]
		if !exists {
			changedFile = &ChangedFile{
				Path:    absolutePath,
				Added:   make([]*ChangedRegion, 0),
				Deleted: make([]*ChangedRegion, 0),
			}
			changedFilesMap[absolutePath] = changedFile
		}

		patch, err := change.Patch()
		if err != nil {
			return ChangedFiles{}, err
		}

		if len(patch.FilePatches()) == 0 {
			continue
		}
		filePatch := patch.FilePatches()[0]
		added, deleted := computeChangedRegions(filePatch.Chunks())
		changedFile.Added = added
		changedFile.Deleted = deleted
	}
	files := make([]*ChangedFile, 0, len(changedFilesMap))
	for _, file := range changedFilesMap {
		files = append(files, file)
	}

	return ChangedFiles{Files: files}, nil
}

func getRepoRoot(repo *git.Repository, err error) (string, error) {
	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %v", err)
	}
	repoRoot := worktree.Filesystem.Root()
	return repoRoot, nil
}

func computeChangedRegions(chunks []diff.Chunk) ([]*ChangedRegion, []*ChangedRegion) {
	var toLine = 1
	var added = make([]*ChangedRegion, 0, len(chunks))
	var deleted = make([]*ChangedRegion, 0, len(chunks))
	for _, chunk := range chunks {
		lines := splitLines(chunk.Content())
		nLines := len(lines)
		switch chunk.Type() {
		case diff.Equal:
			// same line in origin and in modified
			toLine += nLines
		case diff.Delete:
			// deleted from the origin file
			deleted = append(deleted, &ChangedRegion{FirstLine: toLine, Count: nLines})
		case diff.Add:
			// added to the new file
			added = append(added, &ChangedRegion{FirstLine: toLine, Count: nLines})
			toLine += nLines
		}
	}
	return added, deleted
}

var lineSplitter = regexp.MustCompile(`[^\n]*(\n|$)`)

func splitLines(s string) []string {
	ret := lineSplitter.FindAllString(s, -1)
	if ret[len(ret)-1] == "" {
		ret = ret[:len(ret)-1]
	}
	return ret
}
