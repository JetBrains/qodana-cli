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
	"bufio"
	"fmt"
	log "github.com/sirupsen/logrus"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type ChangedRegion struct {
	FirstLine int `json:"firstLine"`
	Count     int `json:"count"`
}

type HunkChange struct {
	FromPath string
	ToPath   string
	Added    []*ChangedRegion
	Deleted  []*ChangedRegion
}

type ChangedFile struct {
	Path    string           `json:"path"`
	Added   []*ChangedRegion `json:"added"`
	Deleted []*ChangedRegion `json:"deleted"`
}

type ChangedFiles struct {
	Files []*ChangedFile `json:"files"`
}

func computeAbsCwd(cwd string) (string, error) {
	cwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", err
	}
	cwdAbs, err := filepath.Abs(cwd)
	return cwdAbs, err
}

func GitChangedFiles(cwd string, diffStart string, diffEnd string, logdir string) (ChangedFiles, error) {
	absCwd, err := computeAbsCwd(cwd)
	if err != nil {
		return ChangedFiles{}, err
	}
	repoRoot, err := GitRoot(cwd, logdir)
	if err != nil {
		return ChangedFiles{}, err
	}

	stdout, _, err := gitRun(cwd, []string{"diff", diffStart, diffEnd, "--unified=0", "--no-renames"}, logdir)
	if err != nil {
		return ChangedFiles{}, err
	}
	return parseDiff(stdout, repoRoot, absCwd)
}

// parseDiff parses the git diff output and extracts changes
func parseDiff(diff string, repoRoot string, cwd string) (ChangedFiles, error) {
	log.Debugf("Parsing diff of length: %d, repo root: %s, cwd: %s", len(diff), repoRoot, cwd)
	var changes []HunkChange
	scanner := bufio.NewScanner(strings.NewReader(diff))

	var currentChange *HunkChange
	// Regular expressions to match diff headers and hunks
	reFilename := regexp.MustCompile(`^diff --git a/(.*) b/(.*)`)
	reHunk := regexp.MustCompile(`^@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@`)

	for scanner.Scan() {
		line := scanner.Text()

		if matches := reFilename.FindStringSubmatch(line); matches != nil {
			if currentChange != nil {
				changes = append(changes, *currentChange)
			}
			currentChange = &HunkChange{
				FromPath: matches[1],
				ToPath:   matches[2],
				Added:    []*ChangedRegion{},
				Deleted:  []*ChangedRegion{},
			}
			continue
		}

		if matches := reHunk.FindStringSubmatch(line); matches != nil && currentChange != nil {
			origLineStart := diffToInt(matches[1])
			origCount := diffToInt(matches[2])
			newLineStart := diffToInt(matches[3])
			newCount := diffToInt(matches[4])
			if origCount != 0 {
				currentChange.Deleted = append(currentChange.Deleted, &ChangedRegion{FirstLine: origLineStart, Count: origCount})
			}
			if newCount != 0 {
				currentChange.Added = append(currentChange.Added, &ChangedRegion{FirstLine: newLineStart, Count: newCount})
			}
		}
	}

	if currentChange != nil {
		changes = append(changes, *currentChange)
	}

	if err := scanner.Err(); err != nil {
		return ChangedFiles{}, err
	}

	files := make([]*ChangedFile, 0, len(changes))
	for _, file := range changes {
		fileName := file.ToPath
		if file.ToPath != file.FromPath {
			if len(file.Deleted) > 0 {
				fileName = file.FromPath
			} else {
				fileName = file.ToPath
			}
		}
		path := filepath.Join(repoRoot, fileName)
		if strings.HasPrefix(path, cwd) { // take changes only inside project
			files = append(files, &ChangedFile{
				Path:    path,
				Added:   file.Added,
				Deleted: file.Deleted,
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return ChangedFiles{Files: files}, nil
}

// diffToInt converts a string to an integer preserving git default number logic
func diffToInt(str string) int {
	if str == "" {
		return 1
	}
	var result int
	_, _ = fmt.Sscanf(str, "%d", &result)
	return result
}
