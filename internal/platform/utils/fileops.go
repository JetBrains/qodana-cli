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

package utils

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

// CopyFile copies a file from src to dst.
func CopyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	err = os.WriteFile(dst, input, 0644)
	if err != nil {
		return err
	}
	return nil
}

// AppendToFile appends text to a file.
func AppendToFile(filename string, text string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Error(err)
		}
	}(f)

	if _, err := f.WriteString(text); err != nil {
		return err
	}
	return nil
}

// CopyDir copies a directory from src to dst.
func CopyDir(src string, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	err = os.MkdirAll(dst, srcInfo.Mode())
	if err != nil {
		return err
	}
	directory, _ := os.ReadDir(src)
	for _, item := range directory {
		srcPath := filepath.Join(src, item.Name())
		dstPath := filepath.Join(dst, item.Name())
		if item.IsDir() {
			err = CopyDir(srcPath, dstPath)
			if err != nil {
				return err
			}
		} else {
			err = CopyFile(srcPath, dstPath)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// GetSha256 computes a hash sum for a byte stream.
func GetSha256(stream io.Reader) (result [32]byte, err error) {
	hasher := sha256.New()
	_, err = io.Copy(hasher, stream)
	if err != nil {
		return result, err
	}

	copy(result[:], hasher.Sum(nil))
	return result, nil
}

// GetFileSha256 computes a hash sum from an existing file.
func GetFileSha256(path string) (result [32]byte, err error) {
	reader, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer func() {
		err = errors.Join(err, reader.Close())
	}()

	return GetSha256(reader)
}

// WalkArchiveCallback will be called for each archived item, e.g. in WalkArchive.
// Parameters:
// - path: relative path of an item within the archive
// - info: `os.FileInfo` for the item
// - contents: `io.Reader` for the file contents or `nil` if the `info.IsDir()` is true.
type WalkArchiveCallback = func(path string, info os.FileInfo, contents io.Reader)

// WalkArchive calls `callback` for each file entry in an archive specified by `path`.
// Format is guessed automatically from the extension.
func WalkArchive(path string, callback WalkArchiveCallback) error {
	var implFunc func(path string, callback WalkArchiveCallback) error = nil

	switch filepath.Ext(path) {
	case ".zip", ".sit":
		implFunc = WalkZipArchive
	case ".tgz":
		implFunc = WalkTarGzArchive
	case ".gz":
		switch filepath.Ext(strings.TrimSuffix(path, ".gz")) {
		case ".tar":
			implFunc = WalkTarGzArchive
		}
	}

	if implFunc == nil {
		return fmt.Errorf("path %q does not have a recognizable extension", path)
	}

	return implFunc(path, callback)
}

// WalkZipArchive implements WalkArchive for .zip.
func WalkZipArchive(path string, callback WalkArchiveCallback) (err error) {
	zipReader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, zipReader.Close())
	}()

	for _, f := range zipReader.File {
		if !filepath.IsLocal(f.Name) {
			// path is not safe: either empty, has invalid name, or navigates to outside the parent dir
			return fmt.Errorf("archive %q contains path %q which is not safe to extract", path, f.Name)
		}

		fileInfo := f.FileInfo()

		err = func() (err error) { // wrap file open/close in a scope
			var reader io.ReadCloser = nil
			if !fileInfo.IsDir() {
				reader, err = f.Open()
				if err != nil {
					return err
				}
				defer func() {
					err = errors.Join(err, reader.Close())
				}()
			}

			callback(f.Name, fileInfo, reader)
			return nil
		}()
		if err != nil {
			return err
		}
	}

	return nil
}

// WalkTarGzArchive implements WalkArchive for .tar.gz.
func WalkTarGzArchive(path string, callback WalkArchiveCallback) (err error) {
	reader, err := os.Open(path)
	if err != nil {
		log.Fatalf("Failed to open %q: %s", path, err)
	}
	defer func() {
		err = errors.Join(err, reader.Close())
	}()

	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		log.Fatalf("gzip error in %q: %s", path, err)
	}
	defer func() {
		err = errors.Join(err, gzReader.Close())
	}()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatalf("tar error while reading contents of %q: %s", path, err)
		}
		if header == nil {
			continue
		}

		if !filepath.IsLocal(header.Name) {
			// path is not safe: either empty, has invalid name, or navigates to outside the parent dir
			return fmt.Errorf("archive %q contains path %q which is not safe to extract", path, header.Name)
		}

		fileInfo := header.FileInfo()
		callbackReader := tarReader
		if fileInfo.IsDir() {
			callbackReader = nil
		}

		callback(header.Name, fileInfo, callbackReader)
	}

	return nil
}
