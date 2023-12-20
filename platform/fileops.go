package platform

import (
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
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
