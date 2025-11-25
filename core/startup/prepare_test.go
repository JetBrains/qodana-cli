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

package startup

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/v2025/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	log "github.com/sirupsen/logrus"
)

func initGitRepo(t *testing.T, path string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}
	if err := exec.Command("git", "-C", path, "config", "user.email", "qodana.support@jetbrains.com").Run(); err != nil {
		t.Fatalf("Failed to set git config user.email: %v", err)
	}
	if err := exec.Command("git", "-C", path, "config", "user.name", "Qodana test").Run(); err != nil {
		t.Fatalf("Failed to set git config user.name: %v", err)
	}
}

func createGitCommit(t *testing.T, path string) {
	testFile := filepath.Join(path, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if err := exec.Command("git", "-C", path, "add", ".").Run(); err != nil {
		t.Fatalf("Failed to add files to git: %v", err)
	}
	if err := exec.Command("git", "-C", path, "commit", "-m", "test commit").Run(); err != nil {
		t.Fatalf("Failed to create git commit: %v", err)
	}
}

func TestCheckVcsSameAsRepositoryRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available, skipping test")
	}

	tmpDir := filepath.Join(os.TempDir(), "vcsRootTest")
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(tmpDir)

	for _, tc := range []struct {
		name           string
		projectDir     string
		gitRoot        string
		repositoryRoot string
		expectWarning  bool
	}{
		{
			name:           "VCS root same as repository root - no warning",
			projectDir:     "project1",
			gitRoot:        "project1",
			repositoryRoot: "project1",
			expectWarning:  false,
		},
		{
			name:           "VCS root different from repository root - warning expected",
			projectDir:     "project2/subdir",
			gitRoot:        "project2",
			repositoryRoot: "project2/subdir",
			expectWarning:  true,
		},
		{
			name:           "Nested subdir with git root at top - warning expected",
			projectDir:     "project3/level1/level2",
			gitRoot:        "project3",
			repositoryRoot: "project3/level1/level2",
			expectWarning:  true,
		},
		{
			name:           "RepositoryRoot matches VCS root even if ProjectDir is deeper",
			projectDir:     "project4/src",
			gitRoot:        "project4",
			repositoryRoot: "project4",
			expectWarning:  false,
		},
	} {
		t.Run(
			tc.name, func(t *testing.T) {
				projectDir := filepath.Join(tmpDir, tc.projectDir)
				gitRoot := filepath.Join(tmpDir, tc.gitRoot)
				repositoryRoot := filepath.Join(tmpDir, tc.repositoryRoot)

				err := os.MkdirAll(projectDir, 0o755)
				if err != nil {
					t.Fatal(err)
				}

				initGitRepo(t, gitRoot)
				createGitCommit(t, gitRoot)

				// Resolve symlinks to handle /var -> /private/var on macOS
				projectDirResolved, _ := filepath.EvalSymlinks(projectDir)
				if projectDirResolved == "" {
					projectDirResolved = projectDir
				}
				repositoryRootResolved, _ := filepath.EvalSymlinks(repositoryRoot)
				if repositoryRootResolved == "" {
					repositoryRootResolved = repositoryRoot
				}

				// Capture log output
				var buf bytes.Buffer
				log.SetOutput(&buf)
				defer log.SetOutput(os.Stderr)

				ctx := commoncontext.Context{
					Analyzer:       product.JvmLinter.DockerAnalyzer(),
					ProjectDir:     projectDirResolved,
					RepositoryRoot: repositoryRootResolved,
				}

				checkVcsSameAsRepositoryRoot(ctx)

				logOutput := buf.String()
				hasWarning := strings.Contains(logOutput, "level=warning") &&
					strings.Contains(logOutput, "git root directory is different")

				if tc.expectWarning && !hasWarning {
					t.Errorf("Expected warning but none was found. Log output: %s", logOutput)
				}
				if !tc.expectWarning && hasWarning {
					t.Errorf("Did not expect warning but found one. Log output: %s", logOutput)
				}
			},
		)
	}
}

func TestSyncCacheSyncIdea(t *testing.T) {
	testProjectDir, commonCtx := setupIdeaSyncTestData(t)
	commonCtx.CacheDir = filepath.Join("testdata", "synccache", "syncidea", "cache")

	SyncCache(commonCtx, product.Product{})

	expectedFile := filepath.Join(testProjectDir, ".idea", "cached.xml")
	checkFileExists(t, expectedFile, true)
}

func TestSyncCacheSyncIdeaNoOverwrite(t *testing.T) {
	testProjectDir, commonCtx := setupIdeaSyncTestData(t)
	createIdeaFolderWithUncachedXml(t, testProjectDir)

	commonCtx.CacheDir = filepath.Join("testdata", "synccache", "syncidea", "cache")

	SyncCache(commonCtx, product.Product{})

	expectedFile := filepath.Join(testProjectDir, ".idea", "uncached.xml")
	checkFileExists(t, expectedFile, true)

	unexpectedFile := filepath.Join(testProjectDir, ".idea", "cached.xml")
	checkFileExists(t, unexpectedFile, false)
}

func TestSyncCacheSyncNoCacheNoProblem(t *testing.T) {
	testProjectDir, commonCtx := setupIdeaSyncTestData(t)
	createIdeaFolderWithUncachedXml(t, testProjectDir)

	commonCtx.CacheDir = filepath.Join(os.TempDir(), ".qodana_sync_cache_sync_idea_no_cache2_test")

	SyncCache(commonCtx, product.Product{})

	expectedFile := filepath.Join(testProjectDir, ".idea", "uncached.xml")
	checkFileExists(t, expectedFile, true)
}

func TestSyncCacheSyncConfig(t *testing.T) {
	testCases := []struct {
		name           string
		baseScriptName string
	}{
		{
			name:           "Idea",
			baseScriptName: product.Idea,
		},
		{
			name:           "PyCharm",
			baseScriptName: product.PyCharm,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testJdkTableXmlSync(t, tc)
			testNoOverwriteJdkTableXml(t, tc)
		})
	}
}

func testNoOverwriteJdkTableXml(t *testing.T, tc struct {
	name           string
	baseScriptName string
}) {
	outputJdkTable, commonCtx, prod := setupCommonTestEnvForConfigSync(t, tc)

	err := os.WriteFile(outputJdkTable, []byte("uncached"), 0o644)
	SyncCache(commonCtx, prod)

	content, err := os.ReadFile(outputJdkTable)
	if err != nil {
		t.Fatal(err)
	}

	expected := "uncached"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}
}

func testJdkTableXmlSync(t *testing.T, tc struct {
	name           string
	baseScriptName string
}) {
	outputJdkTable, commonCtx, prod := setupCommonTestEnvForConfigSync(t, tc)

	SyncCache(commonCtx, prod)

	content, err := os.ReadFile(outputJdkTable)
	if err != nil {
		t.Fatal(err)
	}

	expected := "cached"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}
}

func setupIdeaSyncTestData(t *testing.T) (string, commoncontext.Context) {
	testProjectDir := filepath.Join(t.TempDir())
	return testProjectDir, commoncontext.Context{
		ProjectDir: testProjectDir,
	}
}

func checkFileExists(t *testing.T, filePath string, shouldExist bool) {
	_, err := os.Stat(filePath)
	if shouldExist && os.IsNotExist(err) {
		t.Fatalf("file does not exist: %s", filePath)
	}
	if !shouldExist && os.IsExist(err) {
		t.Fatalf("file should not exist: %s", filePath)
	}
}

func createIdeaFolderWithUncachedXml(t *testing.T, testProjectDir string) {
	err := os.MkdirAll(testProjectDir+"/.idea", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(testProjectDir+"/.idea/uncached.xml", []byte{}, 0o755)
	if err != nil {
		t.Fatal(err)
	}
}

func setupCommonTestEnvForConfigSync(t *testing.T, tc struct {
	name           string
	baseScriptName string
}) (string, commoncontext.Context, product.Product) {
	prod := product.Product{BaseScriptName: tc.baseScriptName, Version: "2025.3"}

	testConfDir := t.TempDir()
	err := os.MkdirAll(testConfDir+"/253/config/options", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	outputJdkTable := filepath.Join(testConfDir, "253", "config", "options", "jdk.table.xml")

	cacheDir := filepath.Join("testdata", "synccache", "syncconfig", "cache")

	commonCtx := commoncontext.Context{
		QodanaSystemDir: testConfDir,
		Id:              "253",
		CacheDir:        cacheDir,
	}

	return outputJdkTable, commonCtx, prod
}
