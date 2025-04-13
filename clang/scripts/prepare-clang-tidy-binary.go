// Find the correct archive for this system, rename it to something that go:embed will pick up, and compute its sha-256
// sum.
package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"

	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
)

func main() {
	// find the correct archive to prepare.
	archivePath := fmt.Sprintf("clang-tidy-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		archivePath += ".zip"
	} else {
		archivePath += ".tar.gz"
	}

	// Compute hash for the clang-tidy binary
	hash := ([]byte)(nil)
	callback := func(path string, info os.FileInfo, stream io.Reader) {
		if info.IsDir() {
			return
		}
		if path != "bin/clang-tidy" && path != "bin/clang-tidy.exe" {
			return
		}

		err := (error)(nil)
		hash, err = utils.GetSha256(stream)
		if err != nil {
			log.Fatalf("sha256 error: %s", err)
		}
	}

	utils.WalkArchive(archivePath, callback)
	err := os.WriteFile("clang-tidy.sha256.bin", hash, 0666)
	if err != nil {
		log.Fatal(err)
	}

	// Normalize the input archive name
	utils.CopyFile(archivePath, "clang-tidy.archive")

	_, err = fmt.Fprintf(os.Stderr, "sha256 of the contents of %q: %s\n", archivePath, hex.EncodeToString(hash))
	if err != nil {
		log.Fatal(err)
	}
}
