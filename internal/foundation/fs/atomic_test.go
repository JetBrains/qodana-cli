package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

		// temp file should be gone
		tmpPath := fmt.Sprintf("%s.%d.temp", path, os.Getpid())
		_, err = os.Stat(tmpPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("abort discards temp file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "out.txt")

		w, err := CreateAtomic(path, 0644)
		assert.NoError(t, err)

		_, err = w.Write([]byte("should not persist"))
		assert.NoError(t, err)

		w.Abort()

		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))

		tmpPath := fmt.Sprintf("%s.%d.temp", path, os.Getpid())
		_, err = os.Stat(tmpPath)
		assert.True(t, os.IsNotExist(err))
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
				// All goroutines share the same PID, so they compete on the same temp path.
				// The result should still be a valid, complete file.
				w, err := CreateAtomic(path, 0644)
				if err != nil {
					return // another goroutine may have truncated the temp file
				}
				if _, err := w.Write([]byte(content)); err != nil {
					w.Abort()
					return
				}
				_ = w.Close() // rename may fail if another goroutine renamed first — that's OK
			}()
		}
		wg.Wait()

		data, err := os.ReadFile(path)
		assert.NoError(t, err)
		assert.Equal(t, content, string(data))
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
