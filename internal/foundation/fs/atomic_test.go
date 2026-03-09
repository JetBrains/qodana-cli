package fs

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func noTempFilesLeft(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.temp"))
	assert.NoError(t, err)
	assert.Empty(t, matches, "leftover temp files in %s", dir)
}

func TestCreateAtomic(t *testing.T) {
	t.Run("write and close commits file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "out.txt")

		w, err := CreateAtomic(path, 0644)
		assert.NoError(t, err)

		_, err = w.Write([]byte("hello"))
		assert.NoError(t, err)

		err = w.Close()
		assert.NoError(t, err)

		data, err := os.ReadFile(path)
		assert.NoError(t, err)
		assert.Equal(t, "hello", string(data))

		noTempFilesLeft(t, dir)
	})

	t.Run("abort discards temp file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "out.txt")

		w, err := CreateAtomic(path, 0644)
		assert.NoError(t, err)

		_, err = w.Write([]byte("should not persist"))
		assert.NoError(t, err)

		assert.NoError(t, w.Abort())

		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))

		noTempFilesLeft(t, dir)
	})

	t.Run("concurrent writes produce valid file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "shared.txt")
		content := "the correct content"

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				w, err := CreateAtomic(path, 0644)
				if err != nil {
					return
				}
				if _, err := w.Write([]byte(content)); err != nil {
					_ = w.Abort()
					return
				}
				_ = w.Close()
			}()
		}
		wg.Wait()

		data, err := os.ReadFile(path)
		assert.NoError(t, err)
		assert.Equal(t, content, string(data))

		noTempFilesLeft(t, dir)
	})
}

func TestWriteFileAtomic(t *testing.T) {
	t.Run("writes file atomically", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "out.bin")

		err := WriteFileAtomic(path, []byte{0xDE, 0xAD}, 0644)
		assert.NoError(t, err)

		data, err := os.ReadFile(path)
		assert.NoError(t, err)
		assert.Equal(t, []byte{0xDE, 0xAD}, data)
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "out.txt")

		err := os.WriteFile(path, []byte("old"), 0644)
		assert.NoError(t, err)

		err = WriteFileAtomic(path, []byte("new"), 0644)
		assert.NoError(t, err)

		data, err := os.ReadFile(path)
		assert.NoError(t, err)
		assert.Equal(t, "new", string(data))
	})
}
