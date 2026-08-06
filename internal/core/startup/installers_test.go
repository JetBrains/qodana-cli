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
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/testutil/mockexe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetIde(t *testing.T) {
	product.RequireNightlyAuth(t)

	for _, linter := range product.AllNativeLinters {
		if linter.ProductCode != product.QDCPP {
			if product.IsReleased {
				ide := getIde(linter.NativeAnalyzer())
				if ide == nil {
					t.Fail()
				}
			} else {
				eap := getIde(&product.NativeAnalyzer{Linter: linter, Eap: true})
				if eap == nil {
					t.Fail()
				}
			}
		}
	}
}

func TestDownloadAndInstallIDE(t *testing.T) {
	product.RequireNightlyAuth(t)
	linters := []product.Linter{product.GoLinter}
	for _, linter := range linters {
		DownloadAndInstallIDE(linter, t)
	}
}

func DownloadAndInstallIDE(linter product.Linter, t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tempDir := filepath.Join(homeDir, ".qodana_scan_", "ideTest")
	err = os.RemoveAll(tempDir)
	if err != nil {
		msg.ErrorMessage("Cannot remove previous temp dir: %s", err)
		t.Fail()
	}

	err = os.MkdirAll(tempDir, 0755)
	if err != nil {
		msg.ErrorMessage("Cannot create temp dir: %s", err)
		t.Fail()
	}

	analyzer := linter.NativeAnalyzer()
	ide := downloadAndInstallIDE(analyzer, tempDir, nil)

	if ide == "" {
		msg.ErrorMessage("Cannot install %s", linter.Name)
		t.Fail()
	}
	prodInfo, err := product.ReadIdeProductInfo(ide)
	if err != nil || prodInfo == nil {
		t.Fatalf("Failed to read IDE product info: %v", err)
	}
	prod := product.GuessProduct(ide, analyzer)

	prepareCustomPlugins(prod)
	disabledPluginsFilePath := prod.DisabledPluginsFilePath()
	if _, err := os.Stat(disabledPluginsFilePath); err != nil {
		t.Fatalf("Cannot find disabled plugins file: %s", disabledPluginsFilePath)
	}

	customPluginsFilePath := prod.CustomPluginsPath()
	if _, err := os.Stat(customPluginsFilePath); err != nil {
		t.Fatalf("Cannot find custom plugins folder: %s", customPluginsFilePath)
	}
}

// Create a target directory for extraction
func TestInstallIdeFromZip(t *testing.T) {
	tests := []struct {
		name       string
		useSymlink bool
		dirPattern string
	}{
		{
			name:       "regular directory",
			useSymlink: false,
			dirPattern: "qodana_test",
		},
		{
			name:       "symlink directory",
			useSymlink: true,
			dirPattern: "qodana_test",
		},
		{
			name:       "arch path with space",
			useSymlink: true,
			dirPattern: "qodana _test",
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				// Create a temporary directory for the test
				tempDir, err := os.MkdirTemp("", tt.dirPattern)
				if err != nil {
					t.Fatalf("Failed to create temporary directory: %v", err)
				}
				defer func(path string) {
					_ = os.RemoveAll(path)
				}(tempDir)

				// Create a source directory with test files
				sourceDir := filepath.Join(tempDir, "source")
				err = os.MkdirAll(sourceDir, 0755)
				if err != nil {
					t.Fatalf("Failed to create source directory: %v", err)
				}

				// Create a test file in the source directory
				testFilePath := filepath.Join(sourceDir, "test.txt")
				err = os.WriteFile(testFilePath, []byte("test content"), 0644)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}

				// Create an archive file from the source directory
				zipFilePath := filepath.Join(tempDir, "test.tar.gz")
				cmd := exec.Command("tar", "-cf", zipFilePath, "-C", sourceDir, ".")
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("Failed to create archive file: %v, output: %s", err, string(output))
				}

				// Create a target directory for extraction
				targetDir := filepath.Join(tempDir, "target")
				if tt.useSymlink {
					err := os.MkdirAll(targetDir, 0755)
					if err != nil {
						t.Fatalf("Failed to create folder for symlink: %v", err)
					}
					symlinkDir := filepath.Join(tempDir, "symlink")
					if err := os.Symlink(targetDir, symlinkDir); err != nil {
						t.Fatalf("Failed to create symlink: %v", err)
					}
					targetDir = filepath.Join(symlinkDir, "target")
				}

				// Call the function under test
				err = installIdeFromZip(zipFilePath, targetDir)
				if err != nil {
					t.Fatalf("installIdeFromZip failed: %v", err)
				}

				// Verify that the file was extracted correctly
				extractedFilePath := filepath.Join(targetDir, "test.txt")
				stat, err := os.Stat(extractedFilePath)
				if errors.Is(err, os.ErrNotExist) {
					t.Fatalf("Expected file %s was not extracted", extractedFilePath)
				}
				if runtime.GOOS == "windows" {
					if stat.Mode().Perm() != 0666 {
						t.Errorf("Expected file permissions 0666, got %v", stat.Mode().Perm())
					}
				} else {
					if stat.Mode().Perm() != 0755 {
						t.Errorf("Expected file permissions 0755, got %v", stat.Mode().Perm())
					}
				}

				// Verify the content of the extracted file
				content, err := os.ReadFile(extractedFilePath)
				if err != nil {
					t.Fatalf("Failed to read extracted file: %v", err)
				}
				if string(content) != "test content" {
					t.Fatalf(
						"Extracted file content does not match. Expected 'test content', got '%s'",
						string(content),
					)
				}
			},
		)
	}
}

func sampleArchive(t *testing.T) string {
	tempdir := t.TempDir()
	archivePath := filepath.Join(tempdir, "example.tar.gz")
	out, err := os.Create(archivePath)
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, out.Close())
	}()

	gw := gzip.NewWriter(out)
	defer func() {
		assert.NoError(t, gw.Close())
	}()
	tw := tar.NewWriter(gw)
	defer func() {
		assert.NoError(t, tw.Close())
	}()

	exampleFile := filepath.Join(tempdir, "example.txt")
	err = fs.AppendToFile(exampleFile, "Hello world")
	assert.NoError(t, err)

	exampleFileData, err := os.Open(exampleFile)
	defer func() {
		assert.NoError(t, exampleFileData.Close())
	}()
	assert.NoError(t, err)

	exampleFileStat, err := exampleFileData.Stat()
	assert.NoError(t, err)
	exampleFileTarHeader, err := tar.FileInfoHeader(exampleFileStat, "")
	assert.NoError(t, err)

	err = tw.WriteHeader(exampleFileTarHeader)
	assert.NoError(t, err)

	_, err = io.Copy(tw, exampleFileData)
	assert.NoError(t, err)

	return archivePath
}

func TestExtractArchive(t *testing.T) {
	archive := sampleArchive(t)

	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "extracted")
	err := extractArchive(archive, targetDir, 0)
	assert.NoError(t, err)

	assert.DirExists(t, targetDir)

	exampleFile := filepath.Join(targetDir, "example.txt")
	assert.FileExists(t, exampleFile)

	contents, err := os.ReadFile(exampleFile)
	assert.NoError(t, err)
	assert.Equal(t, string(contents), "Hello world")
}

func TestExtractArchiveBadPath(t *testing.T) {
	archive := sampleArchive(t)

	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "extracted")

	t.Setenv("PATH", "")
	err := extractArchive(archive, targetDir, 0)
	assert.Error(t, err)
}

func TestIsCustomPluginsCacheValid(t *testing.T) {
	pluginsUrl := "https://example.com/qodana-QDJVM-262.9643.87-custom-plugins.zip"

	seedCache := func(t *testing.T, targetDir string, url string, withPlugin bool) {
		t.Helper()
		pluginsDir := filepath.Join(targetDir, "custom-plugins")
		assert.NoError(t, os.MkdirAll(pluginsDir, 0o755))
		if withPlugin {
			assert.NoError(t, os.WriteFile(filepath.Join(pluginsDir, "plugin.jar"), []byte("jar"), 0o644))
		}
		assert.NoError(t, os.WriteFile(filepath.Join(targetDir, "disabled_plugins.txt"), []byte("id\n"), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(targetDir, customPluginsSourceFile), []byte(url), 0o644))
	}

	t.Run("missing cache", func(t *testing.T) {
		assert.False(t, isCustomPluginsCacheValid(t.TempDir(), pluginsUrl))
	})

	t.Run("valid cache", func(t *testing.T) {
		targetDir := t.TempDir()
		seedCache(t, targetDir, pluginsUrl, true)
		assert.True(t, isCustomPluginsCacheValid(targetDir, pluginsUrl))
	})

	t.Run("url mismatch", func(t *testing.T) {
		targetDir := t.TempDir()
		seedCache(t, targetDir, pluginsUrl, true)
		assert.False(t, isCustomPluginsCacheValid(targetDir, pluginsUrl+"-other"))
	})

	t.Run("legacy cache without source marker", func(t *testing.T) {
		targetDir := t.TempDir()
		assert.NoError(t, os.MkdirAll(filepath.Join(targetDir, "custom-plugins"), 0o755))
		assert.NoError(t, os.WriteFile(filepath.Join(targetDir, "custom-plugins", "plugin.jar"), []byte("jar"), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(targetDir, "disabled_plugins.txt"), []byte("id\n"), 0o644))
		assert.False(t, isCustomPluginsCacheValid(targetDir, pluginsUrl))
	})

	t.Run("empty custom-plugins directory", func(t *testing.T) {
		targetDir := t.TempDir()
		seedCache(t, targetDir, pluginsUrl, false)
		assert.False(t, isCustomPluginsCacheValid(targetDir, pluginsUrl))
	})

	t.Run("custom-plugins is a file", func(t *testing.T) {
		targetDir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(targetDir, "custom-plugins"), []byte("not-a-dir"), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(targetDir, "disabled_plugins.txt"), []byte("id\n"), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(targetDir, customPluginsSourceFile), []byte(pluginsUrl), 0o644))
		assert.False(t, isCustomPluginsCacheValid(targetDir, pluginsUrl))
	})
}

func TestGetPluginsURL(t *testing.T) {
	assert.Equal(t, "https://example.com/ide-custom-plugins.zip", getPluginsURL("https://example.com/ide.sit"))
	assert.Equal(t, "https://example.com/ide-custom-plugins.zip", getPluginsURL("https://example.com/ide-aarch64.sit"))
	assert.Equal(t, "https://example.com/ide-custom-plugins.zip", getPluginsURL("https://example.com/ide.win.zip"))
	assert.Equal(t, "https://example.com/ide-custom-plugins.zip", getPluginsURL("https://example.com/ide.tar.gz"))
}

func captureLogWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logBuf bytes.Buffer
	prevOut := log.StandardLogger().Out
	prevLevel := log.GetLevel()
	log.SetOutput(&logBuf)
	log.SetLevel(log.WarnLevel)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetLevel(prevLevel)
	})
	return &logBuf
}

func TestDownloadCustomPlugins(t *testing.T) {
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS != "darwin" {
		t.Skip("downloadCustomPlugins uses macOS tar zip extraction")
	}

	// Create a zip archive containing custom-plugins/disabled_plugins.txt
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "plugins.zip")
	zipFile, err := os.Create(archivePath)
	assert.NoError(t, err)
	zw := zip.NewWriter(zipFile)
	w, err := zw.Create("custom-plugins/disabled_plugins.txt")
	assert.NoError(t, err)
	_, err = w.Write([]byte("disabled.plugin.id\n"))
	assert.NoError(t, err)
	assert.NoError(t, zw.Close())
	assert.NoError(t, zipFile.Close())

	archiveBytes, err := os.ReadFile(archivePath)
	assert.NoError(t, err)

	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(archiveBytes)))
		_, _ = w.Write(archiveBytes)
	}))
	defer server.Close()

	// downloadCustomPlugins is only called on macOS, where IDE URLs use .sit extensions.
	// DownloadFile issues HEAD then GET, so one logical download is two HTTP requests.
	t.Run(".sit", func(t *testing.T) {
		downloads.Store(0)
		targetDir := filepath.Join(t.TempDir(), "plugins")
		err := downloadCustomPlugins(server.URL+"/ide.sit", targetDir, nil)
		assert.NoError(t, err)
		assert.FileExists(t, filepath.Join(targetDir, "disabled_plugins.txt"))
		assert.FileExists(t, filepath.Join(targetDir, customPluginsSourceFile))
		assert.NoFileExists(t, filepath.Join(targetDir, "custom-plugins.zip"))
		assert.Equal(t, int32(2), downloads.Load())

		err = downloadCustomPlugins(server.URL+"/ide.sit", targetDir, nil)
		assert.NoError(t, err)
		assert.Equal(t, int32(2), downloads.Load(), "second call should use cache")
	})
	t.Run("-aarch64.sit", func(t *testing.T) {
		downloads.Store(0)
		targetDir := filepath.Join(t.TempDir(), "plugins")
		err := downloadCustomPlugins(server.URL+"/ide-aarch64.sit", targetDir, nil)
		assert.NoError(t, err)
		assert.FileExists(t, filepath.Join(targetDir, "disabled_plugins.txt"))
		assert.Equal(t, int32(2), downloads.Load())
	})
	t.Run("re-downloads when url changes", func(t *testing.T) {
		downloads.Store(0)
		targetDir := filepath.Join(t.TempDir(), "plugins")
		assert.NoError(t, downloadCustomPlugins(server.URL+"/ide-262.1.sit", targetDir, nil))
		assert.Equal(t, int32(2), downloads.Load())

		assert.NoError(t, downloadCustomPlugins(server.URL+"/ide-262.2.sit", targetDir, nil))
		assert.Equal(t, int32(4), downloads.Load())
		source, err := os.ReadFile(filepath.Join(targetDir, customPluginsSourceFile))
		assert.NoError(t, err)
		assert.Equal(t, server.URL+"/ide-262.2-custom-plugins.zip", strings.TrimSpace(string(source)))
	})
	t.Run("keeps previous cache when download fails", func(t *testing.T) {
		targetDir := filepath.Join(t.TempDir(), "plugins")
		assert.NoError(t, downloadCustomPlugins(server.URL+"/ide-262.1.sit", targetDir, nil))
		assert.FileExists(t, filepath.Join(targetDir, "disabled_plugins.txt"))
		assert.DirExists(t, filepath.Join(targetDir, "custom-plugins"))

		failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unavailable", http.StatusInternalServerError)
		}))
		defer failing.Close()

		logBuf := captureLogWarnings(t)
		err := downloadCustomPlugins(failing.URL+"/ide-262.2.sit", targetDir, nil)
		assert.Error(t, err)
		assert.Contains(t, logBuf.String(), "keeping previously cached plugins")
		assert.FileExists(t, filepath.Join(targetDir, "disabled_plugins.txt"))
		assert.DirExists(t, filepath.Join(targetDir, "custom-plugins"))
		source, readErr := os.ReadFile(filepath.Join(targetDir, customPluginsSourceFile))
		assert.NoError(t, readErr)
		assert.Equal(t, server.URL+"/ide-262.1-custom-plugins.zip", strings.TrimSpace(string(source)))
		assert.NoFileExists(t, filepath.Join(targetDir, "custom-plugins.old"))
		matches, globErr := filepath.Glob(filepath.Join(targetDir, "custom-plugins.*.partial"))
		assert.NoError(t, globErr)
		assert.Empty(t, matches)
	})
	t.Run("logs failure on first install", func(t *testing.T) {
		failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unavailable", http.StatusInternalServerError)
		}))
		defer failing.Close()

		logBuf := captureLogWarnings(t)
		err := downloadCustomPlugins(failing.URL+"/ide.sit", filepath.Join(t.TempDir(), "plugins"), nil)
		assert.Error(t, err)
		assert.Contains(t, logBuf.String(), "Failed to update custom plugins")
		assert.NotContains(t, logBuf.String(), "keeping previously cached plugins")
	})
	t.Run("logs failure when archive is corrupt", func(t *testing.T) {
		bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload := []byte("not-a-zip")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
			_, _ = w.Write(payload)
		}))
		defer bad.Close()

		logBuf := captureLogWarnings(t)
		err := downloadCustomPlugins(bad.URL+"/ide.sit", filepath.Join(t.TempDir(), "plugins"), nil)
		assert.Error(t, err)
		assert.Contains(t, logBuf.String(), "Failed to update custom plugins")
		assert.NotContains(t, logBuf.String(), "keeping previously cached plugins")
	})
	t.Run("removes stale staging directories", func(t *testing.T) {
		targetDir := filepath.Join(t.TempDir(), "plugins")
		assert.NoError(t, os.MkdirAll(targetDir, 0o755))
		stale := filepath.Join(targetDir, "custom-plugins.12345.partial")
		assert.NoError(t, os.MkdirAll(filepath.Join(stale, "junk"), 0o755))
		assert.NoError(t, os.WriteFile(filepath.Join(stale, "junk", "x"), []byte("x"), 0o644))

		assert.NoError(t, downloadCustomPlugins(server.URL+"/ide.sit", targetDir, nil))
		matches, err := filepath.Glob(filepath.Join(targetDir, "custom-plugins.*.partial"))
		assert.NoError(t, err)
		assert.Empty(t, matches)
		assert.FileExists(t, filepath.Join(targetDir, customPluginsSourceFile))
	})
	t.Run("aborts when previous plugins cannot be removed", func(t *testing.T) {
		targetDir := filepath.Join(t.TempDir(), "plugins")
		assert.NoError(t, downloadCustomPlugins(server.URL+"/ide-262.1.sit", targetDir, nil))
		oldMarker, err := os.ReadFile(filepath.Join(targetDir, customPluginsSourceFile))
		assert.NoError(t, err)

		// Leftover backup that cannot be deleted blocks the update before any swap.
		backupPlugins := filepath.Join(targetDir, "custom-plugins.old")
		nested := filepath.Join(backupPlugins, "nested")
		assert.NoError(t, os.MkdirAll(nested, 0o755))
		assert.NoError(t, os.WriteFile(filepath.Join(nested, "stuck.txt"), []byte("x"), 0o644))
		assert.NoError(t, os.Chmod(nested, 0o000))
		t.Cleanup(func() {
			_ = os.Chmod(nested, 0o755)
			_ = os.RemoveAll(backupPlugins)
		})

		logBuf := captureLogWarnings(t)
		err = downloadCustomPlugins(server.URL+"/ide-262.2.sit", targetDir, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to remove leftover custom plugins backup")
		assert.Contains(t, logBuf.String(), "keeping previously cached plugins")
		source, readErr := os.ReadFile(filepath.Join(targetDir, customPluginsSourceFile))
		assert.NoError(t, readErr)
		assert.Equal(t, string(oldMarker), string(source), "URL marker must stay unchanged")
		assert.DirExists(t, filepath.Join(targetDir, "custom-plugins"))
	})
	t.Run("rolls back when old plugins cannot be deleted after swap", func(t *testing.T) {
		targetDir := filepath.Join(t.TempDir(), "plugins")
		assert.NoError(t, downloadCustomPlugins(server.URL+"/ide-262.1.sit", targetDir, nil))
		oldMarker, err := os.ReadFile(filepath.Join(targetDir, customPluginsSourceFile))
		assert.NoError(t, err)

		// Once moved to custom-plugins.old, this nested dir cannot be removed.
		nested := filepath.Join(targetDir, "custom-plugins", "nested")
		assert.NoError(t, os.MkdirAll(nested, 0o755))
		assert.NoError(t, os.WriteFile(filepath.Join(nested, "stuck.txt"), []byte("x"), 0o644))
		assert.NoError(t, os.Chmod(nested, 0o000))
		t.Cleanup(func() {
			_ = os.Chmod(filepath.Join(targetDir, "custom-plugins", "nested"), 0o755)
			_ = os.Chmod(filepath.Join(targetDir, "custom-plugins.old", "nested"), 0o755)
			_ = os.RemoveAll(filepath.Join(targetDir, "custom-plugins.old"))
		})

		logBuf := captureLogWarnings(t)
		err = downloadCustomPlugins(server.URL+"/ide-262.2.sit", targetDir, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to remove previous custom plugins")
		assert.Contains(t, logBuf.String(), "keeping previously cached plugins")
		source, readErr := os.ReadFile(filepath.Join(targetDir, customPluginsSourceFile))
		assert.NoError(t, readErr)
		assert.Equal(t, string(oldMarker), string(source), "URL marker must stay unchanged")
		assert.DirExists(t, filepath.Join(targetDir, "custom-plugins", "nested"))
		assert.NoDirExists(t, filepath.Join(targetDir, "custom-plugins.old"))
	})
}

func TestInstallIdeWindowsExe(t *testing.T) {
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS != "windows" {
		t.Skip("installIdeWindowsExe is Windows-only")
	}

	tmpDir := t.TempDir()
	var invoked atomic.Bool
	fakeExe := mockexe.CreateMockExe(t, filepath.Join(tmpDir, "installer.exe"), func(ctx *mockexe.CallContext) int {
		invoked.Store(true)
		return 0
	})

	targetDir := filepath.Join(tmpDir, "installed")
	assert.NoError(t, os.MkdirAll(targetDir, 0o755))

	err := installIdeWindowsExe(fakeExe, targetDir)
	assert.NoError(t, err)
	assert.True(t, invoked.Load(), "installer mock was not invoked")
}
