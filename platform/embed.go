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
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/tooling"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	converterJar = "intellij-report-converter.jar"
	fuserJar     = "qodana-fuser.jar"
	baselineCli  = "baseline-cli.jar"
)

// extractUtils mounts the helper tools to the temporary folder.
func extractUtils(linter ThirdPartyLinter, cacheDir string, isCommunity bool) (string, MountInfo) {
	tempMountPath, err := getTempDir()
	if err != nil {
		log.Fatal(err)
	}
	permanentMountPath := getToolsMountPath(cacheDir)

	javaPath, err := getJavaExecutablePath()
	if err != nil {
		log.Fatalf("failed to get java executable path: %w", err)
	}

	customTools, err := linter.MountTools(tempMountPath, permanentMountPath, isCommunity)
	if err != nil {
		cleanupUtils(tempMountPath)
		log.Fatal(err)
	}

	converter := ProcessAuxiliaryTool(converterJar, "converter", tempMountPath, permanentMountPath, tooling.Converter)
	fuser := ProcessAuxiliaryTool(fuserJar, "FUS", permanentMountPath, tempMountPath, tooling.Fuser)
	baselineCliJar := ProcessAuxiliaryTool(
		baselineCli,
		"baseline-cli",
		tempMountPath,
		permanentMountPath,
		tooling.BaselineCli,
	)

	mountInfo := MountInfo{
		Converter:   converter,
		Fuser:       fuser,
		BaselineCli: baselineCliJar,
		CustomTools: customTools,
		JavaPath:    javaPath,
	}
	return tempMountPath, mountInfo
}

func getToolsMountPath(cacheDir string) string {
	mountPath := filepath.Join(cacheDir, shortVersion)
	if _, err := os.Stat(mountPath); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(mountPath, 0755)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	return mountPath
}

// cleanupUtils removes the temporary folder with extracted helper tools.
func cleanupUtils(tempMountPath string) {
	if _, err := os.Stat(tempMountPath); err != nil {
		if os.IsNotExist(err) {
			return
		}
	}

	if err := os.RemoveAll(tempMountPath); err != nil {
		log.Fatal("Failed to remove temporary folder")
	}
}

func ProcessAuxiliaryTool(toolName, moniker, tempMountPath string, mountPath string, bytes []byte) string {
	toolPath := filepath.Join(mountPath, toolName)
	if _, err := os.Stat(toolPath); err != nil {
		if os.IsNotExist(err) {
			err := os.WriteFile(toolPath, bytes, 0644)
			if err != nil { // change the second parameter depending on which tool you have to process
				cleanupUtils(tempMountPath)
				log.Fatalf("Failed to write %s : %s", moniker, err)
			}
		}
	}
	return toolPath
}

func getTempDir() (string, error) {
	tmpDir, err := os.MkdirTemp("", "qodana-platform")
	if err != nil {
		return "", err
	}
	return tmpDir, nil
}

func Decompress(archivePath string, destPath string) error {
	isZip := strings.HasSuffix(archivePath, ".zip")
	if //goland:noinspection GoBoolExpressions
	isZip || runtime.GOOS == "windows" {
		err, done := unpackZip(archivePath, destPath)
		if done {
			return err
		}
	} else {
		err, done := extractTarGz(archivePath, destPath)
		if done {
			return err
		}
	}

	return nil
}

// unpackZip unpacks zip archive to the destination path
func unpackZip(archivePath string, destPath string) (error, bool) {
	zipReader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err, true
	}
	defer func(zipReader *zip.ReadCloser) {
		err = zipReader.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(zipReader)

	for _, f := range zipReader.File {
		fpath := filepath.Join(destPath, f.Name)

		// Check for Path Traversal
		if !strings.HasPrefix(fpath, filepath.Clean(destPath)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path", fpath), true
		}

		if f.FileInfo().IsDir() {
			err = os.MkdirAll(fpath, os.ModePerm)
			if err != nil {
				return err, true
			}
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err, true
		}

		dst, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err, true
		}

		src, err := f.Open()
		if err != nil {
			return err, true
		}

		_, err = io.Copy(dst, src)
		if err != nil {
			return err, true
		}

		err = dst.Close()
		if err != nil {
			return err, true
		}
		err = src.Close()
		if err != nil {
			return err, true
		}
	}
	return nil, false
}

// extractTarGz extracts tar.gz archive to the destination path
func extractTarGz(archivePath string, destPath string) (error, bool) {
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return err, true
	}
	defer func(archiveFile *os.File) {
		err := archiveFile.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(archiveFile)

	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		return err, true
	}
	defer func(gzipReader *gzip.Reader) {
		err := gzipReader.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(gzipReader)

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err, true
		}

		target := filepath.Join(destPath, header.Name)
		if !isInDirectory(destPath, target) {
			return fmt.Errorf("%s: illegal file path", target), true
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err = os.MkdirAll(target, 0755); err != nil {
					return err, true
				}
			}
		case tar.TypeReg:
			file, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err, true
			}
			if _, err := io.Copy(file, tarReader); err != nil {
				err := file.Close()
				if err != nil {
					return err, true
				}
				return err, true
			}
			err = file.Close()
			if err != nil {
				return err, true
			}
		}
	}
	return nil, false
}

// isInDirectory checks if the target file is within the destination directory.
func isInDirectory(destPath string, target string) bool {
	relative, err := filepath.Rel(destPath, target)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(relative, "..")
}
