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

func GitDiffNameOnly(cwd string, diffStart string, diffEnd string, logdir string) ([]string, error) {
	stdout, _, err := gitRun(cwd, []string{"diff", "--name-only", "--relative", diffStart, diffEnd}, logdir)
	if err != nil {
		return []string{""}, err
	}
	relPaths := strings.Split(strings.TrimSpace(stdout), "\n")
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
	return absPaths, nil
}

func GitChangedFiles(cwd string, diffStart string, diffEnd string, logdir string) (ChangedFiles, error) {
	stdout, _, err := gitRun(cwd, []string{"diff", diffStart, diffEnd, "--unified=0", "--no-renames"}, logdir)
	if err != nil {
		return ChangedFiles{}, err
	}
	return parseDiff(stdout)
}

// parseDiff parses the git diff output and extracts changes
func parseDiff(diff string) (ChangedFiles, error) {
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
			origLineStart := toInt(matches[1])
			newLineStart := toInt(matches[3])

			var addCount, removeCount int
			for scanner.Scan() {
				line = scanner.Text()
				if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "diff ") {
					addCount = persistAdd(addCount, currentChange, newLineStart)
					removeCount = persistDelete(removeCount, currentChange, origLineStart)
					if matches = reFilename.FindStringSubmatch(line); matches != nil {
						changes = append(changes, *currentChange)
						currentChange = &HunkChange{
							FromPath: matches[1],
							ToPath:   matches[2],
							Added:    []*ChangedRegion{},
							Deleted:  []*ChangedRegion{},
						}
					}
					break
				}
				if strings.HasPrefix(line, "\\") {
					// Handle \ No newline at end of file
					continue
				}
				if strings.HasPrefix(line, "+") {
					removeCount = persistDelete(removeCount, currentChange, origLineStart)
					addCount++
					newLineStart++
				} else if strings.HasPrefix(line, "-") {
					addCount = persistAdd(addCount, currentChange, newLineStart)
					removeCount++
					origLineStart++
				} else {
					addCount = persistAdd(addCount, currentChange, newLineStart)
					removeCount = persistDelete(removeCount, currentChange, origLineStart)
					if matches = reHunk.FindStringSubmatch(line); matches != nil {
						origLineStart = toInt(matches[1])
						newLineStart = toInt(matches[3])
					} else {
						origLineStart++
						newLineStart++
					}
				}
			}
			// Add any remaining counts after loop
			addCount = persistAdd(addCount, currentChange, newLineStart)
			removeCount = persistDelete(removeCount, currentChange, origLineStart)
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
		files = append(files, &ChangedFile{
			Path:    fileName,
			Added:   file.Added,
			Deleted: file.Deleted,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return ChangedFiles{Files: files}, nil
}

func persistDelete(removeCount int, currentChange *HunkChange, origLineStart int) int {
	if removeCount > 0 {
		currentChange.Deleted = append(currentChange.Deleted, &ChangedRegion{FirstLine: origLineStart - removeCount, Count: removeCount})
		removeCount = 0
	}
	return removeCount
}

func persistAdd(addCount int, currentChange *HunkChange, newLineStart int) int {
	if addCount > 0 {
		currentChange.Added = append(currentChange.Added, &ChangedRegion{FirstLine: newLineStart - addCount, Count: addCount})
		addCount = 0
	}
	return addCount
}

// toInt converts a string to an integer
func toInt(str string) int {
	if str == "" {
		return 0
	}
	var result int
	_, _ = fmt.Sscanf(str, "%d", &result)
	return result
}
