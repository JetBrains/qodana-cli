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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type TestConfig struct {
	initialContent  string
	modifiedContent string
	action          string
	result          string
	projDir         string
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
          "firstLine": 5,
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
		{
			initialContent:  "Hello, New File!\nThis file is newly created.\n",
			modifiedContent: "Hello, New File!\nThis file is newly created.\n",
			action:          "rename",
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
		{
			initialContent:  "Hello, New File!\nThis file is newly created.\n",
			modifiedContent: "",
			action:          "subfolder_move",
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
      "path": "subfolder/file2.txt",
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
			modifiedContent: "",
			action:          "subfolder_move",
			projDir:         "subfolder",
			result: `
{
  "files": [
    {
      "path": "subfolder/file2.txt",
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
		t.Run(
			tc.action, func(t *testing.T) {
				temp, _ := os.MkdirTemp("", "")
				repo := createRepo(t, tc)

				defer func(path string) {
					_ = os.RemoveAll(path)
				}(repo)

				repo, err := filepath.EvalSymlinks(repo)
				assert.NoError(t, err)
				projDir := filepath.Join(repo, tc.projDir)
				commits, err := ComputeChangedFiles(projDir, "HEAD~1", "HEAD", temp)

				for _, file := range commits.Files {
					relPath, _ := filepath.Rel(repo, file.Path)
					file.Path = strings.ReplaceAll(relPath, string(os.PathSeparator), "/")
				}
				assert.NoError(t, err)
				jsonCommits, err := json.MarshalIndent(commits, "", "  ")
				assert.NoError(t, err)

				assert.Equal(t, strings.TrimSpace(tc.result), string(jsonCommits))
			},
		)
	}
}

func createRepo(t *testing.T, tc TestConfig) string {
	repo := NewGitRepo(t)

	// File paths
	fileName := "file.txt"
	fileName2 := "file2.txt"

	if tc.action == "subfolder_move" {
		err := os.MkdirAll(filepath.Join(repo.Dir(), "subfolder"), 0755)
		assert.NoError(t, err)
		fileName2 = "subfolder/file2.txt"
	}

	// Create the first file and commit it
	initialFileName := fileName
	if tc.initialContent == "" {
		initialFileName = "file2.txt"
	}
	repo.WriteFile(initialFileName, tc.initialContent)
	repo.CommitAll("initial")

	// Perform the action specified
	switch tc.action {
	case "modify":
		repo.WriteFile(fileName, tc.modifiedContent)
	case "move", "subfolder_move":
		repo.Run("mv", fileName, fileName2)
	case "delete":
		err := os.Remove(filepath.Join(repo.Dir(), fileName))
		assert.NoError(t, err)
	case "rename":
		err := os.Rename(filepath.Join(repo.Dir(), fileName), filepath.Join(repo.Dir(), fileName2))
		assert.NoError(t, err)
	case "create":
		repo.WriteFile(fileName, tc.modifiedContent)
	}

	repo.CommitAll(tc.action + " file")
	return repo.Dir()
}
