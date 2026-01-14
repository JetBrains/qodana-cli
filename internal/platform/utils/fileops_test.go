package utils

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestGetSha256(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	content := []byte("test content")
	_ = os.WriteFile(file, content, 0644)

	f, _ := os.Open(file)
	defer func() { _ = f.Close() }()

	hash, err := GetSha256(f)
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestGetFileSha256(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	content := []byte("test content")
	_ = os.WriteFile(file, content, 0644)

	hash, err := GetFileSha256(file)
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)

	_, err = GetFileSha256("/nonexistent/file.txt")
	assert.Error(t, err)
}

func TestWalkZipArchive(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")

	zipFile, _ := os.Create(zipPath)
	w := zip.NewWriter(zipFile)

	f, _ := w.Create("file1.txt")
	_, _ = f.Write([]byte("content1"))

	f, _ = w.Create("subdir/file2.txt")
	_, _ = f.Write([]byte("content2"))

	_ = w.Close()
	_ = zipFile.Close()

	var files []string
	err := WalkZipArchive(zipPath, func(path string, info os.FileInfo, contents io.Reader) {
		files = append(files, path)
	})

	assert.NoError(t, err)
	assert.Contains(t, files, "file1.txt")
	assert.Contains(t, files, "subdir/file2.txt")
}

func TestWalkTarGzArchive(t *testing.T) {
	dir := t.TempDir()
	tgzPath := filepath.Join(dir, "test.tar.gz")

	file, _ := os.Create(tgzPath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	_ = tarWriter.WriteHeader(&tar.Header{
		Name: "file1.txt",
		Mode: 0644,
		Size: 8,
	})
	_, _ = tarWriter.Write([]byte("content1"))

	_ = tarWriter.WriteHeader(&tar.Header{
		Name: "subdir/file2.txt",
		Mode: 0644,
		Size: 8,
	})
	_, _ = tarWriter.Write([]byte("content2"))

	_ = tarWriter.Close()
	_ = gzWriter.Close()
	_ = file.Close()

	var files []string
	err := WalkTarGzArchive(tgzPath, func(path string, info os.FileInfo, contents io.Reader) {
		files = append(files, path)
	})

	assert.NoError(t, err)
	assert.Contains(t, files, "file1.txt")
	assert.Contains(t, files, "subdir/file2.txt")
}

func TestWalkArchive(t *testing.T) {
	t.Run("zip archive", func(t *testing.T) {
		dir := t.TempDir()
		zipPath := filepath.Join(dir, "test.zip")

		zipFile, _ := os.Create(zipPath)
		w := zip.NewWriter(zipFile)
		f, _ := w.Create("file.txt")
		_, _ = f.Write([]byte("content"))
		_ = w.Close()
		_ = zipFile.Close()

		var called bool
		err := WalkArchive(zipPath, func(path string, info os.FileInfo, contents io.Reader) {
			called = true
		})

		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("tar.gz archive", func(t *testing.T) {
		dir := t.TempDir()
		tgzPath := filepath.Join(dir, "test.tar.gz")

		file, _ := os.Create(tgzPath)
		gzWriter := gzip.NewWriter(file)
		tarWriter := tar.NewWriter(gzWriter)
		_ = tarWriter.WriteHeader(&tar.Header{Name: "file.txt", Size: 7})
		_, _ = tarWriter.Write([]byte("content"))
		_ = tarWriter.Close()
		_ = gzWriter.Close()
		_ = file.Close()

		var called bool
		err := WalkArchive(tgzPath, func(path string, info os.FileInfo, contents io.Reader) {
			called = true
		})

		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("unsupported format", func(t *testing.T) {
		err := WalkArchive("test.rar", func(path string, info os.FileInfo, contents io.Reader) {})
		assert.Error(t, err)
	})
}
