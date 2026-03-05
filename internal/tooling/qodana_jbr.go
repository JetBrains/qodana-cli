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

	"github.com/codeclysm/extract/v4"
)

//go:generate go run scripts/download-qodana-jbr.go

var (
	qodanaJBRPathOnce sync.Once
	qodanaJBRPath     string
)

func GetQodanaJBRPath(cacheDir string) string {
	qodanaJBRPathOnce.Do(
		func() {
			path, err := computeQodanaJbrExecutablePath(cacheDir)
			if err != nil {
				log.Fatalf("failed to compute Qodana JBR path: %v", err)
			}
			qodanaJBRPath = path
		},
	)
	return qodanaJBRPath
}

// computeQodanaJbrExecutablePath detects the system's GOOS/GOARCH, unpacks the appropriate JRE,
// and returns the path to the java executable
func computeQodanaJbrExecutablePath(cacheDir string) (string, error) {
	goos := runtime.GOOS

	embeddedArchivePath := findEmbeddedArchive(embeddedJBR)
	jbrCacheDir := filepath.Join(cacheDir, "qodana-jbr")
	archiveName := filepath.Base(embeddedArchivePath)
	extractDir := filepath.Join(jbrCacheDir, strings.TrimSuffix(archiveName, ".tar.gz"))

	if _, err := os.Stat(extractDir); os.IsNotExist(err) {
		file, err := embeddedJBR.Open(embeddedArchivePath)
		if err != nil {
			return "", fmt.Errorf("failed to open embedded JBR: %w", err)
		}
		defer func(file fs.File) {
			err := file.Close()
			if err != nil {
				log.Printf("WARN: failed to close embedded JBR: %v", err)
			}
		}(file)

		if err := extractTarGzReader(file, extractDir); err != nil {
			return "", fmt.Errorf("failed to extract embedded JBR: %w", err)
		}
	}

	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return "", fmt.Errorf("failed to read extracted JBR directory: %w", err)
	}

	var jbrRoot string
	for _, entry := range entries {
		if entry.IsDir() && (strings.Contains(entry.Name(), "jbrsdk") || strings.Contains(entry.Name(), "jbr")) {
			jbrRoot = filepath.Join(extractDir, entry.Name())
			break
		}
	}

	if jbrRoot == "" {
		return "", fmt.Errorf("failed to find JBR root directory in extracted archive")
	}

	var javaExec string
	switch goos {
	case "windows":
		javaExec = filepath.Join(jbrRoot, "bin", "java.exe")
	case "darwin":
		javaExec = filepath.Join(jbrRoot, "Contents", "Home", "bin", "java")
		if _, err := os.Stat(javaExec); err == nil {
			return javaExec, nil
		}
		javaExec = filepath.Join(jbrRoot, "bin", "java")
	default:
		javaExec = filepath.Join(jbrRoot, "bin", "java")
	}

	if _, err := os.Stat(javaExec); err == nil {
		return javaExec, nil
	}

	return "", fmt.Errorf("failed to find java executable at expected location: %s", javaExec)
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
