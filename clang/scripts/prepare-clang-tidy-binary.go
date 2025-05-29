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
	var hash [32]byte
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

	stat, err := os.Stat(archivePath)
	if err != nil {
		log.Fatal(err)
	}
	if stat.Size() == 0 {
		// Assume someone does not have clt.zip and just `touch`-ed it to proceed with the build.
		_, err = fmt.Fprintf(os.Stderr, "%q is a 0-byte file, will generate mock hashsum.\n", archivePath)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		err = utils.WalkArchive(archivePath, callback)
		if err != nil {
			log.Fatal(err)
		}
	}

	err = os.WriteFile("clang-tidy.sha256.bin", hash[:], 0666)
	if err != nil {
		log.Fatal(err)
	}

	// Normalize the input archive name
	err = utils.CopyFile(archivePath, "clang-tidy.archive")
	if err != nil {
		log.Fatal(err)
	}

	_, err = fmt.Fprintf(os.Stderr, "sha256 of the contents of %q: %s\n", archivePath, hex.EncodeToString(hash[:]))
	if err != nil {
		log.Fatal(err)
	}
}
