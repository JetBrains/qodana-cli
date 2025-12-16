package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// tempFs creates a filesystem of empty files and dirs according to the relative paths that it should contain.
func tempFs(t *testing.T, paths []string) fs.FS {
	root := t.TempDir()

	for _, path := range paths {
		path = root + "/" + path
		parents, filename := filepath.Split(path)

		if parents != "" {
			require.NoError(t, os.MkdirAll(parents, os.ModePerm))
		}

		if filename != "" {
			require.NoError(t, Touch(path))
		}
	}

	return os.DirFS(root)
}

func TestSubDir(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		root := tempFs(t, []string{
			"a/b/c",
		})

		a, err := SubDir(root, "a")
		require.NoError(t, err)

		_, err = SubDir(a, "b")
		require.NoError(t, err)

		_, err = SubDir(root, "a/b")
		require.NoError(t, err)
	})

	t.Run("empty directory", func(t *testing.T) {
		root := tempFs(t, []string{
			"a/b/",
		})

		a, err := SubDir(root, "a")
		require.NoError(t, err)
		require.NotNil(t, a)

		b, err := SubDir(a, "b")
		require.NoError(t, err)
		require.NotNil(t, b)

		b2, err := SubDir(root, "a/b")
		require.NoError(t, err)
		require.NotNil(t, b2)
	})

	t.Run("path does not exist", func(t *testing.T) {
		root := tempFs(t, []string{
			"a/b/c",
		})

		val, err := SubDir(root, "b")
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Nil(t, val)

		val, err = SubDir(root, "a/x")
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Nil(t, val)

		val, err = SubDir(root, "a/b/x")
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Nil(t, val)
	})

	t.Run("path is not a directory", func(t *testing.T) {
		root := tempFs(t, []string{
			"a/b/c",
			"file",
		})

		b, err := SubDir(root, "a/b")
		require.NoError(t, err)
		c, err := SubDir(b, "c")
		require.Error(t, err)
		require.Nil(t, c)

		c2, err := SubDir(root, "a/b/c")
		require.Error(t, err)
		require.Nil(t, c2)

		file, err := SubDir(root, "file")
		require.Error(t, err)
		require.Nil(t, file)
	})
}

func TestTouch(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		path := dir + "/file"

		require.NoError(t, Touch(path))

		stat1, err := os.Stat(path)
		require.NoError(t, err)

		require.NoError(t, Touch(path))

		stat2, err := os.Stat(path)
		require.NoError(t, err)

		require.Greater(t, stat2.ModTime(), stat1.ModTime())
	})
}
