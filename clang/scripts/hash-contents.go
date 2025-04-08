package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

func getSha256(stream io.Reader) (result []byte, err error) {
	hasher := sha256.New()
	_, err = io.Copy(hasher, stream)
	if err != nil {
		return nil, err
	}

	return hasher.Sum(nil), nil
}

func withUncompressedZip(path string, callback func(io.Reader)) (err error) {
	// Call `callback` with the contents of the .zip archive.
	//
	// The function will return an error if the archive contains more than a single file. Directory structure is
	// ignored entirely.
	zipReader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, zipReader.Close())
	}()

	callbackCalled := false

	for _, f := range zipReader.File {
		if f.FileInfo().IsDir() {
			continue // ignore directories
		}

		if !callbackCalled {
			reader, err := f.Open()
			if err != nil {
				return err
			}
			defer func() {
				err = errors.Join(err, reader.Close())
			}()

			callback(reader)
			callbackCalled = true
		} else {
			return fmt.Errorf("archive %q contains more than one file", path)
		}
	}

	if !callbackCalled {
		return fmt.Errorf("archive %q does not contain any files", path)
	}

	return nil
}

func withUncompressedTarGz(path string, callback func(io.Reader)) error {
	// Call `callback` with the contents of the .tar.gz archive.
	//
	// The function will return an error if the archive contains more than a single file. Directory structure is
	// ignored entirely.
	reader, err := os.Open(path)
	if err != nil {
		log.Fatalf("Failed to open %q: %s", path, err)
	}
	defer reader.Close()

	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		log.Fatalf("gzip error in %q: %s", path, err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	callbackCalled := false

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatalf("tar error while reading contents of %q: %s", path, err)
		}
		if header.FileInfo().IsDir() {
			continue // ignore directories
		}

		if !callbackCalled {
			callback(tarReader)
			callbackCalled = true
		} else {
			return fmt.Errorf("archive %q contains more than one file", path)
		}
	}

	if !callbackCalled {
		return fmt.Errorf("archive %q does not contain any files", path)
	}

	return nil
}

func main() {
	if len(os.Args) <= 1 {
		log.Fatal("Expected an archive path as the first command-line argument.")
	}

	archivePath := os.Args[1]
	hash := ([]byte)(nil)
	extension := ""

	callback := func(stream io.Reader) {
		err := (error)(nil)
		hash, err = getSha256(stream)
		if err != nil {
			log.Fatalf("sha256 error: %s", err)
		}
	}

	if strings.HasSuffix(archivePath, ".zip") {
		extension = ".zip"
		err := withUncompressedZip(archivePath, callback)
		if err != nil {
			log.Fatal(err)
		}
	} else if strings.HasSuffix(archivePath, ".tar.gz") {
		extension = ".tar.gz"
		err := withUncompressedTarGz(archivePath, callback)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatalf("Path %q does not have a recognizable extension.", archivePath)
	}

	outPath := archivePath[0:len(archivePath)-len(extension)] + ".sha256.bin"
	err := os.WriteFile(outPath, hash, 0666)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Fprintf(os.Stderr, "sha256 of the contents of %q: %s\n", archivePath, hex.EncodeToString(hash))
}
