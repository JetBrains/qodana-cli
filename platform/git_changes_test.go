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
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type TestConfig struct {
	initialContent  string
	modifiedContent string
	action          string // Either "create" or "delete"
	result          string
}

func TestChangesCalculation(t *testing.T) {
	testCases := []TestConfig{
		{
			initialContent: `
1
2
3
4`,
			modifiedContent: `
1
5
2
3
4
6`,
			action: "modify",
			result: `
{
  "files": [
    {
      "path": "file.txt",
      "added": [
        {
          "firstLine": 3,
          "count": 1
        },
        {
          "firstLine": 6,
          "count": 2
        }
      ],
      "deleted": [
        {
          "firstLine": 6,
          "count": 1
        }
      ]
    }
  ]
}`,
		},
		{
			initialContent:  "Hello, World!\nThis file will be deleted.\n",
			modifiedContent: "",
			action:          "delete",
			result: `
{
  "files": [
    {
      "path": "file.txt",
      "added": [],
      "deleted": [
        {
          "firstLine": 1,
          "count": 2
        }
      ]
    }
  ]
}
`,
		},
		{
			initialContent:  "",
			modifiedContent: "Hello, New File!\nThis file is newly created.\n",
			action:          "create",
			result: `
{
  "files": [
    {
      "path": "file.txt",
      "added": [
        {
          "firstLine": 1,
          "count": 2
        }
      ],
      "deleted": []
    }
  ]
}`,
		},
		{
			initialContent:  "Hello, New File!\nThis file is newly created.\n",
			modifiedContent: "Hello, New File!\nThis file is newly created.\n",
			action:          "move",
			result: `
{
  "files": [
    {
      "path": "file.txt",
      "added": [],
      "deleted": [
        {
          "firstLine": 1,
          "count": 2
        }
      ]
    },
    {
      "path": "file2.txt",
      "added": [
        {
          "firstLine": 1,
          "count": 2
        }
      ],
      "deleted": []
    }
  ]
}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.action, func(t *testing.T) {
			repo := createRepo(t, tc)
			defer func(path string) {
				_ = os.RemoveAll(path)
			}(repo)

			commits, err := GitChangedFiles(repo, "HEAD~1", "HEAD")
			for _, file := range commits.Files {
				file.Path = filepath.Base(file.Path)
			}
			assert.NoError(t, err)
			jsonCommits, err := json.MarshalIndent(commits, "", "  ")
			assert.NoError(t, err)

			assert.Equal(t, strings.TrimSpace(tc.result), string(jsonCommits))
		})
	}
}

func createRepo(t *testing.T, tc TestConfig) string {
	// Step 1: Create a new directory for the repository
	repoDir, err := os.MkdirTemp("", "testrepo")
	assert.NoError(t, err)

	// Step 2: Initialize a new Git repository
	cmd := exec.Command("git", "init")
	runGit(t, cmd, repoDir)

	// File name
	fileName := "file.txt"
	fileName2 := "file2.txt"
	absolutePath := filepath.Join(repoDir, fileName)
	absolutePath2 := filepath.Join(repoDir, fileName2)

	// Step 3: Create the first file and commit it if initial content is not empty
	initialFileName := fileName
	if tc.initialContent == "" {
		initialFileName = "file2.txt"
	}
	err = os.WriteFile(filepath.Join(repoDir, initialFileName), []byte(tc.initialContent), 0644)
	assert.NoError(t, err)

	cmd = exec.Command("git", "add", initialFileName)
	runGit(t, cmd, repoDir)

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	runGit(t, cmd, repoDir)

	// Step 4: Perform the action specified
	switch tc.action {
	case "modify":
		err = os.WriteFile(absolutePath, []byte(tc.modifiedContent), 0644)
		assert.NoError(t, err)
	case "move":
		cmd = exec.Command("git", "mv", absolutePath, absolutePath2)
		runGit(t, cmd, repoDir)
	case "delete":
		err = os.Remove(absolutePath)
		assert.NoError(t, err)
	case "create":
		err = os.WriteFile(absolutePath, []byte(tc.modifiedContent), 0644)
		assert.NoError(t, err)
	}

	// Step 5: Stage changes and commit
	cmd = exec.Command("git", "add", "-A")
	runGit(t, cmd, repoDir)

	cmd = exec.Command("git", "commit", "-m", tc.action+" file")
	runGit(t, cmd, repoDir)

	return repoDir
}

func runGit(t *testing.T, cmd *exec.Cmd, repoDir string) {
	cmd.Dir = repoDir
	err := cmd.Run()
	out, _ := cmd.CombinedOutput()
	log.Info(string(out))
	assert.NoError(t, err)
}
