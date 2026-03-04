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

package qdyaml

import (
	"os"
	"path/filepath"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func setupTestFile(fileName string, content string) {
	tempDir := os.TempDir()
	tempFile := filepath.Join(tempDir, fileName)

	// create a test file with provided content and filename
	file, err := os.Create(tempFile)
	if err != nil {
		log.Fatal(err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(file)
	_, err = file.WriteString(content)
	if err != nil {
		log.Fatal(err)
	}
	err = file.Sync()
	if err != nil {
		log.Fatal(err)
	}
}

func TestLoadQodanaYaml(t *testing.T) {
	testCases := []struct {
		description string
		setup       func(name string)
		project     string
		filename    string
		expected    QodanaYaml
	}{
		{
			description: "file exists but is empty",
			setup: func(name string) {
				setupTestFile(name, "")
			},
			project:  os.TempDir(),
			filename: "empty.yaml",
			expected: QodanaYaml{},
		},
		{
			description: "file exists with valid content",
			setup: func(name string) {
				content := `version: 1.0`
				setupTestFile(name, content)
			},
			project:  os.TempDir(),
			filename: "valid.yaml",
			expected: QodanaYaml{
				Version: "1.0",
			},
		},
		{
			description: "file exists with .net section",
			setup: func(name string) {
				content := `version: 1.0
dotnet:
  project: test.csproj
  frameworks: "!netstandard2.0;!netstandard2.1"`
				setupTestFile(name, content)
			},
			project:  os.TempDir(),
			filename: "dotnet.yaml",
			expected: QodanaYaml{
				Version: "1.0",
				DotNet: DotNet{
					Project:    "test.csproj",
					Frameworks: "!netstandard2.0;!netstandard2.1",
				},
			},
		},
		{
			description: "file exists with script section",
			setup: func(name string) {
				content := `
version: 1.0
profile:
  name: qodana.starter
script:
  name: migrate-classes
  parameters:
    include-mapping: "Java EE to Jakarta EE"
    mapping:
      - old-name: "javax.management.j2ee"
        new-name: "jakarta.management.j2ee"
        type: "package"
        recursive: "true"
      - old-name: "javax.annotation.security.DenyAll"
        new-name: "jakarta.annotation.security.DenyAll"
        type: "class"`
				setupTestFile(name, content)
			},
			project:  os.TempDir(),
			filename: "script.yaml",
			expected: QodanaYaml{
				Version: "1.0",
				Profile: Profile{
					Name: "qodana.starter",
				},
				Script: Script{
					Name: "migrate-classes", Parameters: map[string]any{
						"include-mapping": "Java EE to Jakarta EE",
						"mapping": []any{
							map[string]any{
								"old-name":  "javax.management.j2ee",
								"new-name":  "jakarta.management.j2ee",
								"type":      "package",
								"recursive": "true",
							},
							map[string]any{
								"old-name": "javax.annotation.security.DenyAll",
								"new-name": "jakarta.annotation.security.DenyAll",
								"type":     "class",
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(
			tc.description, func(t *testing.T) {
				tc.setup(tc.filename)
				actual := TestOnlyLoadLocalNotEffectiveQodanaYaml(tc.project, tc.filename)
				_ = os.Remove(filepath.Join(tc.project, tc.filename))
				assert.Equal(t, tc.expected, actual)
			},
		)
	}
}

func TestDotNet_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		dotnet   DotNet
		expected bool
	}{
		{"empty", DotNet{}, true},
		{"solution set", DotNet{Solution: "test.sln"}, false},
		{"project set", DotNet{Project: "test.csproj"}, false},
		{"both set", DotNet{Solution: "test.sln", Project: "test.csproj"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.dotnet.IsEmpty())
		})
	}
}

func TestGetLocalNotEffectiveQodanaYamlFullPath(t *testing.T) {
	t.Run("absolute path", func(t *testing.T) {
		projectDir := t.TempDir()
		absPath := filepath.Join(t.TempDir(), "abs", "path", "qodana.yaml")
		result := GetLocalNotEffectiveQodanaYamlFullPath(projectDir, absPath)
		assert.Equal(t, absPath, result)
	})

	t.Run("relative path", func(t *testing.T) {
		projectDir := t.TempDir()
		result := GetLocalNotEffectiveQodanaYamlFullPath(projectDir, "qodana.yaml")
		assert.Equal(t, filepath.Join(projectDir, "qodana.yaml"), result)
	})

	t.Run("empty path no yaml", func(t *testing.T) {
		dir := t.TempDir()
		result := GetLocalNotEffectiveQodanaYamlFullPath(dir, "")
		assert.Equal(t, "", result)
	})

	t.Run("empty path with yaml", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "qodana.yaml"), []byte("version: 1.0"), 0644); err != nil {
			t.Fatal(err)
		}
		result := GetLocalNotEffectiveQodanaYamlFullPath(dir, "")
		assert.Equal(t, filepath.Join(dir, "qodana.yaml"), result)
	})
}

func TestLoadQodanaYamlByFullPath(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		result := LoadQodanaYamlByFullPath("")
		assert.Equal(t, QodanaYaml{}, result)
	})

	t.Run("non-existent file", func(t *testing.T) {
		nonExistentPath := filepath.Join(t.TempDir(), "nonexistent", "path", "qodana.yaml")
		result := LoadQodanaYamlByFullPath(nonExistentPath)
		assert.Equal(t, QodanaYaml{}, result)
	})

	t.Run("valid file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "qodana.yaml")
		if err := os.WriteFile(path, []byte("version: \"1.0\"\nlinter: jetbrains/qodana-jvm"), 0644); err != nil {
			t.Fatal(err)
		}
		result := LoadQodanaYamlByFullPath(path)
		assert.Equal(t, "1.0", result.Version)
		assert.Equal(t, "jetbrains/qodana-jvm", result.Linter)
	})
}

func TestQodanaYaml_Sort(t *testing.T) {
	q := &QodanaYaml{
		Includes: []Clude{
			{Name: "Zebra"},
			{Name: "Alpha"},
			{Name: "Beta"},
		},
		Excludes: []Clude{
			{Name: "Zulu"},
			{Name: "Alpha"},
		},
		LicenseRules: []LicenseRule{
			{
				Keys:       []string{"zlib", "apache-2.0", "MIT"},
				Allowed:    []string{"GPL-3.0", "BSD-3-Clause"},
				Prohibited: []string{"Proprietary", "Commercial"},
			},
		},
	}

	q.Sort()

	assert.Equal(t, "Alpha", q.Includes[0].Name)
	assert.Equal(t, "Beta", q.Includes[1].Name)
	assert.Equal(t, "Zebra", q.Includes[2].Name)
	assert.Equal(t, "Alpha", q.Excludes[0].Name)
	assert.Equal(t, "Zulu", q.Excludes[1].Name)
	assert.Equal(t, []string{"apache-2.0", "MIT", "zlib"}, q.LicenseRules[0].Keys)
	assert.Equal(t, []string{"BSD-3-Clause", "GPL-3.0"}, q.LicenseRules[0].Allowed)
	assert.Equal(t, []string{"Commercial", "Proprietary"}, q.LicenseRules[0].Prohibited)
}

func TestQodanaYaml_IsDotNet(t *testing.T) {
	tests := []struct {
		name     string
		yaml     QodanaYaml
		expected bool
	}{
		{"empty", QodanaYaml{}, false},
		{"dotnet linter", QodanaYaml{Linter: "jetbrains/qodana-dotnet"}, true},
		{"cdnet linter", QodanaYaml{Linter: "jetbrains/qodana-cdnet"}, true},
		{"QDNET ide", QodanaYaml{Ide: "QDNET"}, true},
		{"jvm linter", QodanaYaml{Linter: "jetbrains/qodana-jvm"}, false},
		{"go linter", QodanaYaml{Linter: "jetbrains/qodana-go"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.yaml.IsDotNet())
		})
	}
}

func TestSetQodanaDotNet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qodana.yaml")
	if err := os.WriteFile(path, []byte("version: \"1.0\""), 0644); err != nil {
		t.Fatal(err)
	}

	dotNet := &DotNet{
		Solution: "test.sln",
		Project:  "test.csproj",
	}

	result := SetQodanaDotNet(path, dotNet)
	assert.True(t, result)

	loaded := LoadQodanaYamlByFullPath(path)
	assert.Equal(t, "test.sln", loaded.DotNet.Solution)
	assert.Equal(t, "test.csproj", loaded.DotNet.Project)
}
