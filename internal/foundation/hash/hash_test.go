package hash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSha256(t *testing.T) {
	f := strings.NewReader("test content")
	h, err := GetSha256(f)
	assert.NoError(t, err)
	assert.NotEmpty(t, h)
}

func TestGetFileSha256(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	_ = os.WriteFile(file, []byte("test content"), 0644)

	h, err := GetFileSha256(file)
	assert.NoError(t, err)
	assert.NotEmpty(t, h)

	_, err = GetFileSha256("/nonexistent/file.txt")
	assert.Error(t, err)
}

func TestGetSha512(t *testing.T) {
	f := strings.NewReader("test content")
	h, err := GetSha512(f)
	assert.NoError(t, err)
	assert.NotEmpty(t, h)
	// SHA-512 produces 64 bytes
	assert.Len(t, h, 64)
}

func TestGetFileSha512(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	_ = os.WriteFile(file, []byte("test content"), 0644)

	h, err := GetFileSha512(file)
	assert.NoError(t, err)
	assert.NotEmpty(t, h)

	_, err = GetFileSha512("/nonexistent/file.txt")
	assert.Error(t, err)
}

func TestSha256Determinism(t *testing.T) {
	content := "deterministic content"
	h1, _ := GetSha256(strings.NewReader(content))
	h2, _ := GetSha256(strings.NewReader(content))
	assert.Equal(t, h1, h2)
}
