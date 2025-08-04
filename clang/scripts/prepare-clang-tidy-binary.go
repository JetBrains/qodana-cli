// Find the correct archive for this system, rename it to something that go:embed will pick up, and compute its sha-256
// sum.
package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"strings"

	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
)

func processArchive(targetOs string, targetArch string) {
	// find the correct archive to prepare.
	archivePath := fmt.Sprintf("clang-tidy-%s-%s", targetOs, targetArch)
	if targetOs == "windows" {
		archivePath += ".zip"
	} else {
		archivePath += ".tar.gz"
	}

	// Compute hash for the clang-tidy binary
	var hash [32]byte
	hashedPath := ""
	callback := func(path string, info os.FileInfo, stream io.Reader) {
		if info.IsDir() {
			return
		}
		if path != "bin/clang-tidy" && path != "bin/clang-tidy.exe" {
			return
		}

		hashedPath = path
		err := (error)(nil)
		hash, err = utils.GetSha256(stream)
		if err != nil {
			log.Fatalf("sha256 error: %s", err)
		}
	}

	stat, err := os.Stat(archivePath)
	if errors.Is(err, fs.ErrNotExist) {
		_, err = fmt.Fprintf(os.Stderr, "skipping archive %q: file does not exist\n", archivePath)
		if err != nil {
			log.Fatal(err)
		}
		return
	}
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
		if hashedPath == "" {
			log.Fatalf("Could not find a clang-tidy executable in %q", archivePath)
		}
	}

	hashFile := fmt.Sprintf("%s.sha256.bin", archivePath)
	err = os.WriteFile(hashFile, hash[:], 0666)
	if err != nil {
		log.Fatal(err)
	}

	_, err = fmt.Fprintf(os.Stderr, "sha256 of %s/%s: %s\n", archivePath, hashedPath, hex.EncodeToString(hash[:]))
	if err != nil {
		log.Fatal(err)
	}
}

var TARGET_PLATFORMS = []string{
	"darwin/amd64",
	"darwin/arm64",
	"linux/amd64",
	"linux/arm64",
	"windows/amd64",
	"windows/arm64",
}

func main() {
	for _, platform := range TARGET_PLATFORMS {
		platformParts := strings.Split(platform, "/")
		processArchive(platformParts[0], platformParts[1])
	}
}
