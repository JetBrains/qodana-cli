package fs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyFile copies a file from src to dst using streaming (not loading entire file into memory).
func CopyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, in.Close())
	}()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, out.Close())
	}()

	_, err = io.Copy(out, in)
	return err
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

// AppendToFile appends text to a file.
func AppendToFile(filename string, text string) (err error) {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, f.Close())
	}()

	_, err = f.WriteString(text)
	return err
}

// CheckDirFiles checks if a directory contains any files.
func CheckDirFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// CleanDirectory removes all entries in a directory without removing the directory itself.
func CleanDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("failed to remove %s: %w", p, err)
		}
	}
	return nil
}

// SameFile checks if two paths reference the same file (same inode).
func SameFile(a, b string) bool {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	return os.SameFile(infoA, infoB)
}

// CreateTempDir creates a temporary directory with the given name prefix.
// Returns the path, a cleanup function, and any error.
func CreateTempDir(name string) (string, func(), error) {
	dir, err := os.MkdirTemp("", fmt.Sprintf("%s-", name))
	if err != nil {
		return "", func() {}, err
	}
	cleanupFunc := func() {
		_ = os.RemoveAll(dir)
	}
	return dir, cleanupFunc, nil
}
