package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckDirFiles(t *testing.T) {
	t.Run("dir with files", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
		assert.True(t, CheckDirFiles(dir))
	})

	t.Run("empty dir", func(t *testing.T) {
		dir := t.TempDir()
		assert.False(t, CheckDirFiles(dir))
	})

	t.Run("non-existent dir", func(t *testing.T) {
		assert.False(t, CheckDirFiles("/nonexistent/path"))
	})
}

func TestFindFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file1.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file2.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file3.txt"), []byte("text"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("find go files", func(t *testing.T) {
		files := FindFiles(dir, []string{".go"})
		assert.Len(t, files, 2)
	})

	t.Run("find txt files", func(t *testing.T) {
		files := FindFiles(dir, []string{".txt"})
		assert.Len(t, files, 1)
	})

	t.Run("find multiple extensions", func(t *testing.T) {
		files := FindFiles(dir, []string{".go", ".txt"})
		assert.Len(t, files, 3)
	})

	t.Run("no matching files", func(t *testing.T) {
		files := FindFiles(dir, []string{".py"})
		assert.Empty(t, files)
	})
}

func TestGetDefaultUser(t *testing.T) {
	result := GetDefaultUser()
	if runtime.GOOS == "windows" {
		assert.Equal(t, "root", result)
	} else {
		assert.Contains(t, result, ":")
	}
}

func TestIsInstalled(t *testing.T) {
	t.Run("git is installed", func(t *testing.T) {
		assert.True(t, IsInstalled("git"))
	})

	t.Run("nonexistent command", func(t *testing.T) {
		assert.False(t, IsInstalled("nonexistent_command_xyz"))
	})
}

func TestIsProcess(t *testing.T) {
	assert.False(t, IsProcess("definitely_not_a_real_process_xyz_123"))
}

func TestCopyFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcFile := filepath.Join(srcDir, "source.txt")
	dstFile := filepath.Join(dstDir, "dest.txt")
	content := "test content"

	if err := os.WriteFile(srcFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

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

