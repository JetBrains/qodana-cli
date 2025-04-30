package utils

import (
	"os"
	"path/filepath"
	"runtime"

	"testing"

	"github.com/stretchr/testify/assert"
)

func tempDir(t *testing.T) string {
	dir := t.TempDir()
	result, err := CanonicalPath(dir) // temp dir path is runtime dependent and is not part of tests
	if err != nil {
		t.Fatalf("Failed to make path to temp dir %q canonical: %s", dir, err)
	}
	return result
}

func mkdirp(t *testing.T, path string) {
	err := os.MkdirAll(path, 0o700)
	if err != nil {
		t.Fatalf("Failed to create directory %s: %s", path, err)
	}
}

func touch(t *testing.T, path string) {
	mkdirp(t, filepath.Dir(path))

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create file %s: %s", path, err)
	}
	err = file.Close()
	if err != nil {
		t.Fatalf("Failed to close file %s: %s", path, err)
	}
}

func isTestDirCaseSensitive(t *testing.T) bool {
	dir := t.TempDir()
	dir1 := filepath.Join(dir, "A")
	dir2 := filepath.Join(dir, "a")

	mkdirp(t, dir1)
	_, err := os.Stat(dir2)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		t.Fatalf("Error in Stat(%s): %s", dir2, err)
	}

	return false
}

func symlink(t *testing.T, source string, path string) {
	err := os.Symlink(source, path)
	if err != nil {
		t.Fatalf("Failed to create symlink: %s", err)
	}
}

func TestCanonicalPathNoop(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/file")
	touch(t, expected)
	actual, err := CanonicalPath(expected)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathNotFound(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/missing")
	// touch(t, expected)
	actual, err := CanonicalPath(expected)

	assert.True(t, os.IsNotExist(err))
	assert.Equal(t, actual, "")
}

func TestCanonicalPathCaseInsensitive(t *testing.T) {
	if isTestDirCaseSensitive(t) {
		t.Skip("Not relevant on case-sensitive filesystems")
	}

	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/Aa")
	touch(t, expected)
	actual, err := CanonicalPath(tempDir + filepath.FromSlash("/aA"))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathTrailingSlash(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/dir")
	mkdirp(t, expected)
	actual, err := CanonicalPath(tempDir + filepath.FromSlash("/dir/"))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathMultipleSlashes(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/file")
	touch(t, expected)
	actual, err := CanonicalPath(tempDir + filepath.FromSlash("////file"))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathWrongSlash(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Only relevant to Windows")
	}

	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/file")
	touch(t, expected)
	actual, err := CanonicalPath(tempDir + "/file")

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathDot(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/dir/file")
	touch(t, expected)
	actual, err := CanonicalPath(tempDir + filepath.FromSlash("/dir/./file"))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathTrailingDot(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/dir")
	mkdirp(t, expected)
	actual, err := CanonicalPath(tempDir + filepath.FromSlash("/dir/."))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathDotDot(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/dir")
	mkdirp(t, expected)
	actual, err := CanonicalPath(tempDir + filepath.FromSlash("/dir/../dir"))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathTrailingSymlink(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/dir")
	query := tempDir + filepath.FromSlash("/symlink")
	mkdirp(t, expected)
	symlink(t, "dir", query)
	actual, err := CanonicalPath(query)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathSymlink(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/dir/nested")
	mkdirp(t, expected)
	symlink(t, "dir", tempDir+filepath.FromSlash("/symlink"))
	actual, err := CanonicalPath(tempDir + filepath.FromSlash("/symlink/nested"))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathSymlinkDotDot(t *testing.T) {
	// dir  1
	// dir  1/1.1
	// dir  2
	// link 2/2.1 -> 1/1.1
	tempDir := tempDir(t)
	mkdirp(t, tempDir+filepath.FromSlash("/1/1.1"))
	mkdirp(t, tempDir+filepath.FromSlash("/2"))
	symlink(t, tempDir+filepath.FromSlash("/1/1.1"), tempDir+filepath.FromSlash("/2/2.1"))

	expected := tempDir + filepath.FromSlash("/1")
	actual, err := CanonicalPath(tempDir + filepath.FromSlash("/2/2.1/.."))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathCaseInsenitiveSymlink(t *testing.T) {
	if isTestDirCaseSensitive(t) {
		t.Skip("Not relevant on case-sensitive filesystems")
	}

	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/dir")
	query := tempDir + filepath.FromSlash("/symlink")
	mkdirp(t, expected)
	symlink(t, "DIR", query)
	actual, err := CanonicalPath(query)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathRelative(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/dir")
	mkdirp(t, expected)

	t.Chdir(tempDir)
	actual, err := CanonicalPath("dir")

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalPathRelativeSymlinkDotDot(t *testing.T) {
	// dir  1
	// dir  1/1.1
	// dir  2
	// link 2/2.1 -> 1/1.1
	tempDir := tempDir(t)
	mkdirp(t, tempDir+filepath.FromSlash("/1/1.1"))
	mkdirp(t, tempDir+filepath.FromSlash("/2"))
	symlink(t, tempDir+filepath.FromSlash("/1/1.1"), tempDir+filepath.FromSlash("/2/2.1"))

	expected := tempDir + filepath.FromSlash("/1")

	t.Chdir(tempDir)
	actual, err := CanonicalPath(filepath.FromSlash("2/2.1/.."))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}
