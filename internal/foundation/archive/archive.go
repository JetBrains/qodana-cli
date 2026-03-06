package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// WalkArchiveCallback will be called for each archived item, e.g. in WalkArchive.
// Parameters:
//   - path: relative path of an item within the archive
//   - info: os.FileInfo for the item
//   - contents: io.Reader for the file contents or nil if info.IsDir() is true.
type WalkArchiveCallback = func(path string, info os.FileInfo, contents io.Reader)

// WalkArchive calls callback for each file entry in an archive specified by path.
// Format is guessed automatically from the extension.
func WalkArchive(path string, callback WalkArchiveCallback) error {
	var implFunc func(path string, callback WalkArchiveCallback) error

	switch filepath.Ext(path) {
	case ".zip", ".sit":
		implFunc = WalkZipArchive
	case ".tgz":
		implFunc = WalkTarGzArchive
	case ".gz":
		switch filepath.Ext(strings.TrimSuffix(path, ".gz")) {
		case ".tar":
			implFunc = WalkTarGzArchive
		}
	}

	if implFunc == nil {
		return fmt.Errorf("path %q does not have a recognizable extension", path)
	}

	return implFunc(path, callback)
}

// WalkZipArchive implements WalkArchive for .zip.
func WalkZipArchive(path string, callback WalkArchiveCallback) (err error) {
	zipReader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, zipReader.Close())
	}()

	for _, f := range zipReader.File {
		if !filepath.IsLocal(f.Name) {
			return fmt.Errorf("archive %q contains path %q which is not safe to extract", path, f.Name)
		}

		fileInfo := f.FileInfo()

		err = func() (err error) {
			var reader io.ReadCloser
			if !fileInfo.IsDir() {
				reader, err = f.Open()
				if err != nil {
					return err
				}
				defer func() {
					err = errors.Join(err, reader.Close())
				}()
			}

			callback(f.Name, fileInfo, reader)
			return nil
		}()
		if err != nil {
			return err
		}
	}

	return nil
}

// WalkTarGzArchive implements WalkArchive for .tar.gz.
func WalkTarGzArchive(path string, callback WalkArchiveCallback) (err error) {
	reader, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w", path, err)
	}
	defer func() {
		err = errors.Join(err, reader.Close())
	}()

	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("gzip error in %q: %w", path, err)
	}
	defer func() {
		err = errors.Join(err, gzReader.Close())
	}()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("tar error while reading contents of %q: %w", path, err)
		}

		if !filepath.IsLocal(header.Name) {
			return fmt.Errorf("archive %q contains path %q which is not safe to extract", path, header.Name)
		}

		fileInfo := header.FileInfo()
		callbackReader := tarReader
		if fileInfo.IsDir() {
			callbackReader = nil
		}

		callback(header.Name, fileInfo, callbackReader)
	}

	return nil
}

func isPathWithinDir(path, dir string) bool {
	cleanDir := filepath.Clean(dir)
	cleanPath := filepath.Clean(path)
	return cleanPath == cleanDir || strings.HasPrefix(cleanPath, cleanDir+string(os.PathSeparator))
}

// ExtractTarGz extracts a .tar.gz file to destDir. If stripTopDir is true, the first
// path component is removed from each entry (e.g., "jbrsdk-25.0.2-linux-x64-b329.72/bin/java" -> "bin/java").
func ExtractTarGz(archivePath, destDir string, stripTopDir bool) (err error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", archivePath, err)
	}
	defer func() {
		err = errors.Join(err, f.Close())
	}()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader for %s: %w", archivePath, err)
	}
	defer func() {
		err = errors.Join(err, gz.Close())
	}()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create dest dir %s: %w", destDir, err)
	}
	// Resolve destDir to a canonical path so EvalSymlinks comparisons work
	// on systems where temp dirs contain symlinks (e.g. macOS /var -> /private/var).
	destDir, err = filepath.EvalSymlinks(destDir)
	if err != nil {
		return fmt.Errorf("failed to resolve dest dir %s: %w", destDir, err)
	}

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar %s: %w", archivePath, err)
		}

		name := hdr.Name
		if stripTopDir {
			if idx := strings.IndexByte(name, '/'); idx >= 0 {
				name = name[idx+1:]
			} else {
				continue
			}
			if name == "" {
				continue
			}
		}

		target := filepath.Join(destDir, name)

		if !isPathWithinDir(target, destDir) {
			return fmt.Errorf("tar entry %q attempts path traversal", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if resolved, err := filepath.EvalSymlinks(filepath.Dir(target)); err == nil {
				if !isPathWithinDir(resolved, destDir) {
					return fmt.Errorf("tar entry %q resolves outside dest dir via symlink", hdr.Name)
				}
			}
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)|0o755); err != nil {
				return fmt.Errorf("failed to create dir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("failed to create parent dir for %s: %w", target, err)
			}
			if resolved, err := filepath.EvalSymlinks(filepath.Dir(target)); err == nil {
				if !isPathWithinDir(resolved, destDir) {
					return fmt.Errorf("tar entry %q resolves outside dest dir via symlink", hdr.Name)
				}
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", target, err)
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				return errors.Join(fmt.Errorf("failed to extract %s: %w", target, err), outFile.Close())
			}
			if err := outFile.Close(); err != nil {
				return fmt.Errorf("failed to close %s: %w", target, err)
			}
		case tar.TypeSymlink:
			resolvedLink := filepath.Join(filepath.Dir(target), hdr.Linkname)
			if !isPathWithinDir(resolvedLink, destDir) {
				return fmt.Errorf("tar entry %q has symlink target %q that escapes dest dir", hdr.Name, hdr.Linkname)
			}
			// Symlinks may fail on Windows; best-effort
			_ = os.Symlink(hdr.Linkname, target)
		}
	}
	return nil
}

// CreateTarGz creates a .tar.gz archive from sourceDir. Files inside the archive
// are placed under topDir (e.g., "qodana-jbrsdk-25.0.2-linux-x64-b329.72/...").
func CreateTarGz(sourceDir, archivePath, topDir string) (err error) {
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return fmt.Errorf("failed to create dir for %s: %w", archivePath, err)
	}

	f, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("failed to create archive %s: %w", archivePath, err)
	}
	defer func() {
		err = errors.Join(err, f.Close())
	}()

	gw := gzip.NewWriter(f)
	defer func() {
		err = errors.Join(err, gw.Close())
	}()

	tw := tar.NewWriter(gw)
	defer func() {
		err = errors.Join(err, tw.Close())
	}()

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		entryName := topDir
		if relPath != "." {
			entryName = filepath.ToSlash(filepath.Join(topDir, relPath))
		}

		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}

		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		hdr.Name = entryName

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, file)
		return errors.Join(copyErr, file.Close())
	})
	if err != nil {
		return fmt.Errorf("failed to create tar.gz %s: %w", archivePath, err)
	}
	return nil
}
