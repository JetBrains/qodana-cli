package platform

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/tooling"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	converterJar = "intellij-report-converter.jar"
	fuserJar     = "qodana-fuser.jar"
	baselineCli  = "baseline-cli.jar"
)

// temporary folder for mounting helper tools in community mode
var tempMountPath string

// mount mounts the helper tools to the temporary folder.
func mount(options *QodanaOptions) {
	path, err := getTempDir()
	if err != nil {
		log.Fatal(err)
	}
	linterOptions := options.GetLinterSpecificOptions()
	if linterOptions == nil {
		log.Fatal("Linter options are not defined for 3rd party linter")
	}
	tempMountPath = path
	permanentMountPath := getToolsMountPath(options)

	mountInfo := (*linterOptions).GetMountInfo()
	mountInfo.Converter = ProcessAuxiliaryTool(converterJar, "converter", permanentMountPath, tooling.Converter)
	mountInfo.Fuser = ProcessAuxiliaryTool(fuserJar, "FUS", permanentMountPath, tooling.Fuser)
	mountInfo.BaselineCli = ProcessAuxiliaryTool(baselineCli, "baseline-cli", permanentMountPath, tooling.BaselineCli)
	mountInfo.CustomTools, err = (*linterOptions).MountTools(tempMountPath, permanentMountPath, options)
	if err != nil {
		umount()
		log.Fatal(err)
	}
}

func getToolsMountPath(options *QodanaOptions) string {
	linterInfo := options.GetLinterInfo()
	if linterInfo == nil {
		log.Fatal("Linter info is not defined for 3rd party linter")
	}
	mountPath := filepath.Join(options.cacheDirPath(), (*linterInfo).LinterVersion)
	if _, err := os.Stat(mountPath); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(mountPath, 0755)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	return mountPath
}

// umount removes the temporary folder with extracted helper tools.
func umount() {
	if _, err := os.Stat(tempMountPath); err != nil {
		if os.IsNotExist(err) {
			return
		}
	}

	if err := os.RemoveAll(tempMountPath); err != nil {
		log.Fatal("Failed to remove temporary folder")
	}
}

func ProcessAuxiliaryTool(toolName, moniker, mountPath string, bytes []byte) string {
	toolPath := filepath.Join(mountPath, toolName)
	if _, err := os.Stat(toolPath); err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(toolPath, bytes, 0644); err != nil { // change the second parameter depending on which tool you have to process
				umount()
				log.Fatalf("Failed to write %s : %s", moniker, err)
			}
		}
	}
	return toolPath
}

func getTempDir() (string, error) {
	tmpDir, err := os.MkdirTemp("", "qodana-platform")
	if err != nil {
		return "", err
	}
	return tmpDir, nil
}

func Decompress(archivePath string, destPath string) error {
	isZip := strings.HasSuffix(archivePath, ".zip")
	if //goland:noinspection GoBoolExpressions
	isZip || runtime.GOOS == "windows" {
		err, done := unpackZip(archivePath, destPath)
		if done {
			return err
		}
	} else {
		err, done := extractTarGz(archivePath, destPath)
		if done {
			return err
		}
	}

	return nil
}

// unpackZip unpacks zip archive to the destination path
func unpackZip(archivePath string, destPath string) (error, bool) {
	zipReader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err, true
	}
	defer func(zipReader *zip.ReadCloser) {
		err = zipReader.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(zipReader)

	for _, f := range zipReader.File {
		fpath := filepath.Join(destPath, f.Name)

		// Check for Path Traversal
		if !strings.HasPrefix(fpath, filepath.Clean(destPath)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path", fpath), true
		}

		if f.FileInfo().IsDir() {
			err = os.MkdirAll(fpath, os.ModePerm)
			if err != nil {
				return err, true
			}
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err, true
		}

		dst, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err, true
		}

		src, err := f.Open()
		if err != nil {
			return err, true
		}

		_, err = io.Copy(dst, src)
		if err != nil {
			return err, true
		}

		err = dst.Close()
		if err != nil {
			return err, true
		}
		err = src.Close()
		if err != nil {
			return err, true
		}
	}
	return nil, false
}

// extractTarGz extracts tar.gz archive to the destination path
func extractTarGz(archivePath string, destPath string) (error, bool) {
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return err, true
	}
	defer func(archiveFile *os.File) {
		err := archiveFile.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(archiveFile)

	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		return err, true
	}
	defer func(gzipReader *gzip.Reader) {
		err := gzipReader.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(gzipReader)

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err, true
		}

		target := filepath.Join(destPath, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err = os.MkdirAll(target, 0755); err != nil {
					return err, true
				}
			}
		case tar.TypeReg:
			file, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err, true
			}
			if _, err := io.Copy(file, tarReader); err != nil {
				err := file.Close()
				if err != nil {
					return err, true
				}
				return err, true
			}
			err = file.Close()
			if err != nil {
				return err, true
			}
		}
	}
	return nil, false
}
