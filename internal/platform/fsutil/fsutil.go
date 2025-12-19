package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// SubDir returns a sub-filesystem of an existing filesystem, rooted at the directory specified by path.
func SubDir(root fs.FS, path string) (fs.FS, error) {
	fileinfo, err := fs.Stat(root, path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fs.ErrNotExist
	}
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}

	if !fileinfo.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", path)
	}

	// Interesting fact: fs.Sub returns no error if path does not exist. How fun! :)
	return fs.Sub(root, path)
}

// Touch creates a file if it does not exist and updates the atime and mtime.
func Touch(path string) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()

	now := time.Now()
	return os.Chtimes(path, now, now)
}
