package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
