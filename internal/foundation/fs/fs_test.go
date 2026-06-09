package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "src.txt")
	dstFile := filepath.Join(dir, "dst.txt")
	content := "test content"
	_ = os.WriteFile(srcFile, []byte(content), 0644)

	t.Run("copy file successfully", func(t *testing.T) {
		err := CopyFile(srcFile, dstFile)
		assert.NoError(t, err)

		data, err := os.ReadFile(dstFile)
		assert.NoError(t, err)
		assert.Equal(t, content, string(data))
	})

	t.Run("copy non-existent file", func(t *testing.T) {
		err := CopyFile("/nonexistent/file.txt", dstFile)
		assert.Error(t, err)
	})
}

func TestCopyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	_ = os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644)
	_ = os.Mkdir(filepath.Join(srcDir, "subdir"), 0755)
	_ = os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644)

	err := CopyDir(srcDir, filepath.Join(dstDir, "copied"))
	assert.NoError(t, err)

	data, _ := os.ReadFile(filepath.Join(dstDir, "copied", "file1.txt"))
	assert.Equal(t, "content1", string(data))

	data, _ = os.ReadFile(filepath.Join(dstDir, "copied", "subdir", "file2.txt"))
	assert.Equal(t, "content2", string(data))
}

func TestAppendToFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "append.txt")

	t.Run("append to new file", func(t *testing.T) {
		err := AppendToFile(file, "line1\n")
		assert.NoError(t, err)

		data, _ := os.ReadFile(file)
		assert.Equal(t, "line1\n", string(data))
	})

	t.Run("append to existing file", func(t *testing.T) {
		err := AppendToFile(file, "line2\n")
		assert.NoError(t, err)

		data, _ := os.ReadFile(file)
		assert.Equal(t, "line1\nline2\n", string(data))
	})
}

func TestCheckDirFiles(t *testing.T) {
	t.Run("dir with files", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "test.txt"), []byte("content"), 0644)
		assert.True(t, CheckDirFiles(dir))
	})

	t.Run("empty dir", func(t *testing.T) {
		dir := t.TempDir()
		assert.False(t, CheckDirFiles(dir))
	})

	t.Run("nonexistent dir", func(t *testing.T) {
		assert.False(t, CheckDirFiles("/nonexistent/dir"))
	})
}

func TestCleanDirectory(t *testing.T) {
	t.Run("clean directory with files", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content"), 0644)
		_ = os.Mkdir(filepath.Join(dir, "subdir"), 0755)
		_ = os.WriteFile(filepath.Join(dir, "subdir", "file2.txt"), []byte("content"), 0644)

		err := CleanDirectory(dir)
		assert.NoError(t, err)

		entries, _ := os.ReadDir(dir)
		assert.Empty(t, entries)

		// Directory itself still exists
		_, err = os.Stat(dir)
		assert.NoError(t, err)
	})

	t.Run("clean empty directory", func(t *testing.T) {
		dir := t.TempDir()
		err := CleanDirectory(dir)
		assert.NoError(t, err)
	})

	t.Run("clean nonexistent directory", func(t *testing.T) {
		err := CleanDirectory("/nonexistent/dir")
		assert.NoError(t, err) // should not error
	})
}

func TestSameFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	_ = os.WriteFile(file, []byte("content"), 0644)

	t.Run("same path", func(t *testing.T) {
		assert.True(t, SameFile(file, file))
	})

	t.Run("hard link", func(t *testing.T) {
		link := filepath.Join(dir, "link.txt")
		err := os.Link(file, link)
		if err != nil {
			t.Skip("hard links not supported")
		}
		assert.True(t, SameFile(file, link))
	})

	t.Run("different files", func(t *testing.T) {
		other := filepath.Join(dir, "other.txt")
		_ = os.WriteFile(other, []byte("content"), 0644)
		assert.False(t, SameFile(file, other))
	})

	t.Run("nonexistent file", func(t *testing.T) {
		assert.False(t, SameFile(file, "/nonexistent"))
		assert.False(t, SameFile("/nonexistent", file))
	})
}

func TestCreateTempDir(t *testing.T) {
	dir, cleanup, err := CreateTempDir("test-prefix")
	assert.NoError(t, err)
	assert.DirExists(t, dir)
	assert.Contains(t, filepath.Base(dir), "test-prefix")

	cleanup()
	assert.NoDirExists(t, dir)
}
