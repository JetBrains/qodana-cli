package utils

import (
	"os"
	"path/filepath"

	"testing"

	"github.com/stretchr/testify/assert"
)

func tempDir(t *testing.T) string {
	result, err := Canonical(t.TempDir()) // temp dir path is runtime dependent and is not part of tests
	if err != nil {
		t.Fatalf("Failed to make path to temp dir canonical: %s", err)
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

func TestCanonicalNoop(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/file")
	touch(t, expected)
	actual, err := Canonical(expected)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalNotFound(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/missing")
	// touch(t, expected)
	actual, err := Canonical(expected)

	assert.True(t, os.IsNotExist(err))
	assert.Equal(t, actual, "")
}

func TestCanonicalCase(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/Aa")
	touch(t, expected)
	actual, err := Canonical(tempDir + filepath.FromSlash("/aA"))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalTrailingSlash(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/dir")
	mkdirp(t, expected)
	actual, err := Canonical(tempDir + filepath.FromSlash("/dir/"))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalDot1(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/dir")
	mkdirp(t, expected)
	actual, err := Canonical(tempDir + filepath.FromSlash("/dir/."))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalDot2(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/dir/file")
	touch(t, expected)
	actual, err := Canonical(tempDir + filepath.FromSlash("/dir/./file"))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonicalDotDot(t *testing.T) {
	tempDir := tempDir(t)
	expected := tempDir + filepath.FromSlash("/dir")
	mkdirp(t, expected)
	actual, err := Canonical(tempDir + filepath.FromSlash("/dir/../dir"))

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}
