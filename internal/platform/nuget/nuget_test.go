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

package nuget

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
)

func createTempFileWithContent(content string) string {
	tempFile, _ := os.CreateTemp("", "test")
	_, _ = tempFile.WriteString(content)
	err := tempFile.Close()
	if err != nil {
		return ""
	}
	return tempFile.Name()
}

//goland:noinspection HttpUrlsUsage
func TestCheckForPrivateFeed(t *testing.T) {
	testCases := []struct {
		name     string
		filename string
		expected bool
	}{
		{
			name:     "FileWithPrivateFeed",
			filename: createTempFileWithContent(`<add key= 'xxx' value = 'http://'>`),
			expected: true,
		},
		{
			name:     "FileWithPrivateFeed2",
			filename: createTempFileWithContent(`<add key= 'xxx' value = 'https://'>`),
			expected: true,
		},
		{
			name:     "FileWithoutPrivateFeed",
			filename: createTempFileWithContent(`<add key= 'xxx' value='yyy'>`),
			expected: false,
		},
		{
			name:     "EmptyFile",
			filename: createTempFileWithContent(""),
			expected: false,
		},
	}

	for _, test := range testCases {
		t.Run(
			test.name, func(t *testing.T) {
				result := checkForPrivateFeed(test.filename)
				if result != test.expected {
					t.Errorf("got/want mismatch, got %v, want %v", result, test.expected)
				}
				_ = os.Remove(test.filename)
			},
		)
	}
}

func TestQodanaNugetVarsSet(t *testing.T) {
	// Save original values
	originalUrl := os.Getenv(qdenv.QodanaNugetUrl)
	originalUser := os.Getenv(qdenv.QodanaNugetUser)
	originalPassword := os.Getenv(qdenv.QodanaNugetPassword)
	defer func() {
		_ = os.Setenv(qdenv.QodanaNugetUrl, originalUrl)
		_ = os.Setenv(qdenv.QodanaNugetUser, originalUser)
		_ = os.Setenv(qdenv.QodanaNugetPassword, originalPassword)
	}()

	t.Run("all vars set", func(t *testing.T) {
		_ = os.Setenv(qdenv.QodanaNugetUrl, "https://nuget.example.com")
		_ = os.Setenv(qdenv.QodanaNugetUser, "user")
		_ = os.Setenv(qdenv.QodanaNugetPassword, "password")
		if !qodanaNugetVarsSet() {
			t.Error("expected qodanaNugetVarsSet() to return true when all vars are set")
		}
	})

	t.Run("missing url", func(t *testing.T) {
		_ = os.Setenv(qdenv.QodanaNugetUrl, "")
		_ = os.Setenv(qdenv.QodanaNugetUser, "user")
		_ = os.Setenv(qdenv.QodanaNugetPassword, "password")
		if qodanaNugetVarsSet() {
			t.Error("expected qodanaNugetVarsSet() to return false when url is missing")
		}
	})

	t.Run("missing user", func(t *testing.T) {
		_ = os.Setenv(qdenv.QodanaNugetUrl, "https://nuget.example.com")
		_ = os.Setenv(qdenv.QodanaNugetUser, "")
		_ = os.Setenv(qdenv.QodanaNugetPassword, "password")
		if qodanaNugetVarsSet() {
			t.Error("expected qodanaNugetVarsSet() to return false when user is missing")
		}
	})

	t.Run("missing password", func(t *testing.T) {
		_ = os.Setenv(qdenv.QodanaNugetUrl, "https://nuget.example.com")
		_ = os.Setenv(qdenv.QodanaNugetUser, "user")
		_ = os.Setenv(qdenv.QodanaNugetPassword, "")
		if qodanaNugetVarsSet() {
			t.Error("expected qodanaNugetVarsSet() to return false when password is missing")
		}
	})

	t.Run("all vars empty", func(t *testing.T) {
		_ = os.Setenv(qdenv.QodanaNugetUrl, "")
		_ = os.Setenv(qdenv.QodanaNugetUser, "")
		_ = os.Setenv(qdenv.QodanaNugetPassword, "")
		if qodanaNugetVarsSet() {
			t.Error("expected qodanaNugetVarsSet() to return false when all vars are empty")
		}
	})
}

func TestPrepareNugetConfig(t *testing.T) {
	_ = os.Setenv(qdenv.QodanaNugetName, "qdn")
	_ = os.Setenv(qdenv.QodanaNugetUrl, "test_url")
	_ = os.Setenv(qdenv.QodanaNugetUser, "test_user")
	_ = os.Setenv(qdenv.QodanaNugetPassword, "test_password")

	// create temp dir
	tmpDir, _ := os.MkdirTemp("", "test")
	defer func(tmpDir string) {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
	}(tmpDir)

	PrepareNugetConfig(tmpDir)

	expected := `<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <clear />
    <add key="nuget.org" value="https://api.nuget.org/v3/index.json" />
    <add key="qdn" value="test_url" />
  </packageSources>
  <packageSourceCredentials>
    <qdn>
      <add key="Username" value="test_user" />
      <add key="ClearTextPassword" value="test_password" />
    </qdn>
  </packageSourceCredentials>
</configuration>`

	file, err := os.Open(filepath.Join(tmpDir, ".nuget", "NuGet", "NuGet.Config"))
	if err != nil {
		t.Fatal(err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}(file)

	scanner := bufio.NewScanner(file)
	var text string
	for scanner.Scan() {
		text += scanner.Text() + "\n"
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	text = strings.TrimSuffix(text, "\n")
	if text != expected {
		t.Fatalf("got:\n%s\n\nwant:\n%s", text, expected)
	}
}
