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

package tooling

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/JetBrains/qodana-cli/internal/platform/strutil"
	"github.com/codeclysm/extract/v4"
)

//go:generate go run scripts/download-qodana-jbr.go

var (
	qodanaJBRPathOnce sync.Once
	qodanaJBRPath     string
)

func GetQodanaJBRPath() string {
	qodanaJBRPathOnce.Do(
		func() {
			qodanaJBRPath = strutil.QuoteForWindows(computeQodanaJBRPath()) // computes once per process
		},
	)
	return qodanaJBRPath
}

// computeQodanaJBRPath detects the system's GOOS/GOARCH, unpacks the appropriate JRE,
// and returns the path to the java executable
func computeQodanaJBRPath() string {
	goos := runtime.GOOS

	embeddedArchivePath := findEmbeddedArchive(embeddedJBR)
	jbrCacheDir := getJBRCacheDir()
	archiveName := filepath.Base(embeddedArchivePath)
	extractDir := filepath.Join(jbrCacheDir, strings.TrimSuffix(archiveName, ".tar.gz"))

	javaExec := getJavaExecutablePath(extractDir, goos)
	if _, err := os.Stat(javaExec); err == nil {
		return javaExec
	}

	// Need to extract
	file, err := embeddedJBR.Open(embeddedArchivePath)
	if err != nil {
		log.Fatalf("failed to open embedded JBR: %v", err)
	}
	defer func(file fs.File) {
		err := file.Close()
		if err != nil {
			log.Printf("WARN: failed to close embedded JBR: %v", err)
		}
	}(file)

	if err := extractTarGzReader(file, extractDir); err != nil {
		log.Fatalf("failed to extract embedded JBR: %v", err)
	}

	javaExec = getJavaExecutablePath(extractDir, goos)
	if _, err := os.Stat(javaExec); err == nil {
		return javaExec
	}
	log.Fatalf("failed to find java executable in extracted JBR: %v", err)
	return ""
}

// extractTarGzReader extracts a tar.gz archive from a reader to the specified directory
func extractTarGzReader(reader io.Reader, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create extraction directory: %w", err)
	}

	return extract.Gz(context.Background(), reader, destDir, nil)
}

func findEmbeddedArchive(embedFS embed.FS) string {
	matches, err := fs.Glob(embedFS, "qodana-jbrs/*/*.tar.gz")
	if err != nil {
		log.Fatalf("No JBR file found: %s", err)
	}
	if len(matches) != 1 {
		log.Fatalf("expected exactly 1 embedded JBR archive, got %d: %v", len(matches), matches)
	}
	return matches[0]
}

func getJBRCacheDir() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	return filepath.Join(cacheDir, "JetBrains", "Qodana", "JBR")
}

// getJavaExecutablePath returns the expected path to the java executable
func getJavaExecutablePath(extractDir, goos string) string {
	// Find the JBR directory (typically named like qodana-jbrsdk-25.0.1-*)
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return ""
	}

	var jbrRoot string
	for _, entry := range entries {
		if entry.IsDir() && (strings.Contains(entry.Name(), "jbrsdk") || strings.Contains(entry.Name(), "jbr")) {
			jbrRoot = filepath.Join(extractDir, entry.Name())
			break
		}
	}

	if jbrRoot == "" {
		return ""
	}

	// Construct path based on OS
	var javaExec string
	switch goos {
	case "windows":
		javaExec = filepath.Join(jbrRoot, "bin", "java.exe")
	default: // darwin, linux and others
		javaExec = filepath.Join(jbrRoot, "bin", "java")
	}

	// Check if standard location exists
	if _, err := os.Stat(javaExec); err == nil {
		return javaExec
	}

	// For some macOS distributions, check Contents/Home structure
	if goos == "darwin" {
		altPath := filepath.Join(jbrRoot, "Contents", "Home", "bin", "java")
		if _, err := os.Stat(altPath); err == nil {
			return altPath
		}
	}

	return javaExec
}
