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
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetIde(t *testing.T) {
	//goland:noinspection GoBoolExpressions
	if !product.IsReleased {
		t.Setenv(
			"QD_PRODUCT_INTERNAL_FEED",
			fmt.Sprintf("https://packages.jetbrains.team/files/p/sa/qdist/%s/feed.json", product.ReleaseVersion),
		)
	}

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
				if os.IsNotExist(err) {
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
