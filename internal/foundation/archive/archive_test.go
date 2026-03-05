package archive_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/foundation/archive"
	"github.com/stretchr/testify/assert"
)

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
	err := archive.WalkZipArchive(zipPath, func(path string, info os.FileInfo, contents io.Reader) {
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

	_ = tarWriter.WriteHeader(&tar.Header{Name: "file1.txt", Mode: 0644, Size: 8})
	_, _ = tarWriter.Write([]byte("content1"))

	_ = tarWriter.WriteHeader(&tar.Header{Name: "subdir/file2.txt", Mode: 0644, Size: 8})
	_, _ = tarWriter.Write([]byte("content2"))

	_ = tarWriter.Close()
	_ = gzWriter.Close()
	_ = file.Close()

	var files []string
	err := archive.WalkTarGzArchive(tgzPath, func(path string, info os.FileInfo, contents io.Reader) {
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
		err := archive.WalkArchive(zipPath, func(path string, info os.FileInfo, contents io.Reader) {
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
		err := archive.WalkArchive(tgzPath, func(path string, info os.FileInfo, contents io.Reader) {
			called = true
		})

		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("unsupported format", func(t *testing.T) {
		err := archive.WalkArchive("test.rar", func(path string, info os.FileInfo, contents io.Reader) {})
		assert.Error(t, err)
	})
}

func createTestTarGz(t *testing.T, topDir string) string {
	t.Helper()
	dir := t.TempDir()
	tgzPath := filepath.Join(dir, "test.tar.gz")

	file, err := os.Create(tgzPath)
	assert.NoError(t, err)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Directory entry
	_ = tarWriter.WriteHeader(&tar.Header{Name: topDir + "/", Typeflag: tar.TypeDir, Mode: 0755})
	// File entry
	content := []byte("hello world")
	_ = tarWriter.WriteHeader(&tar.Header{Name: topDir + "/file.txt", Mode: 0644, Size: int64(len(content))})
	_, _ = tarWriter.Write(content)
	// Subdirectory
	_ = tarWriter.WriteHeader(&tar.Header{Name: topDir + "/sub/", Typeflag: tar.TypeDir, Mode: 0755})
	sub := []byte("sub content")
	_ = tarWriter.WriteHeader(&tar.Header{Name: topDir + "/sub/nested.txt", Mode: 0644, Size: int64(len(sub))})
	_, _ = tarWriter.Write(sub)

	_ = tarWriter.Close()
	_ = gzWriter.Close()
	_ = file.Close()
	return tgzPath
}

func TestExtractTarGz(t *testing.T) {
	t.Run("extract with strip top dir", func(t *testing.T) {
		tgzPath := createTestTarGz(t, "mydir")
		destDir := t.TempDir()

		err := archive.ExtractTarGz(tgzPath, destDir, true)
		assert.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(destDir, "file.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "hello world", string(data))

		data, err = os.ReadFile(filepath.Join(destDir, "sub", "nested.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "sub content", string(data))
	})

	t.Run("extract without strip", func(t *testing.T) {
		tgzPath := createTestTarGz(t, "mydir")
		destDir := t.TempDir()

		err := archive.ExtractTarGz(tgzPath, destDir, false)
		assert.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(destDir, "mydir", "file.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "hello world", string(data))
	})

	t.Run("nonexistent archive", func(t *testing.T) {
		err := archive.ExtractTarGz("/nonexistent.tar.gz", t.TempDir(), false)
		assert.Error(t, err)
	})
}

func TestCreateTarGz(t *testing.T) {
	// Create source directory with files
	srcDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("hello"), 0644)
	_ = os.Mkdir(filepath.Join(srcDir, "sub"), 0755)
	_ = os.WriteFile(filepath.Join(srcDir, "sub", "nested.txt"), []byte("world"), 0644)

	outDir := t.TempDir()
	archivePath := filepath.Join(outDir, "output.tar.gz")

	err := archive.CreateTarGz(srcDir, archivePath, "mytop")
	assert.NoError(t, err)
	assert.FileExists(t, archivePath)

	// Verify by extracting
	extractDir := t.TempDir()
	err = archive.ExtractTarGz(archivePath, extractDir, false)
	assert.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(extractDir, "mytop", "file.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "hello", string(data))

	data, err = os.ReadFile(filepath.Join(extractDir, "mytop", "sub", "nested.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "world", string(data))
}
