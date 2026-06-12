//go:build ignore

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

	"github.com/JetBrains/qodana-cli/internal/foundation/archive"
	"github.com/JetBrains/qodana-cli/internal/foundation/hash"
)

func main() {
	targetOs := runtime.GOOS
	if override := os.Getenv("TARGETOS"); override != "" {
		targetOs = override
	}

	targetArch := runtime.GOARCH
	if override := os.Getenv("TARGETARCH"); override != "" {
		targetArch = override
	}

	// find the correct archive to prepare.
	archivePath := fmt.Sprintf("clang-tidy-%s-%s", targetOs, targetArch)
	binaryPath := "bin/clang-tidy"
	if targetOs == "windows" {
		archivePath += ".zip"
		binaryPath += ".exe"
	} else {
		archivePath += ".tar.gz"
	}

	// Compute hash for the clang-tidy binary
	var hashResult [32]byte
	found := false
	callback := func(path string, info os.FileInfo, stream io.Reader) {
		if info.IsDir() {
			return
		}
		if path != binaryPath {
			return
		}

		var err error
		hashResult, err = hash.GetSha256(stream)
		if err != nil {
			log.Fatalf("sha256 error: %s", err)
		}
		found = true
	}

	// The download step (../scripts/download-deps.go) guarantees a real archive or fails; a missing or
	// empty file here means it was skipped (no QODANA_CLI_DEPS_TOKEN) — fail loud rather than mock.
	stat, err := os.Stat(archivePath)
	if err != nil {
		log.Fatalf("%s: the download step must run with QODANA_CLI_DEPS_TOKEN set (%s)", archivePath, err)
	}
	if stat.Size() == 0 {
		log.Fatalf("%s is empty; the download step must run with QODANA_CLI_DEPS_TOKEN set", archivePath)
	}
	if err := archive.WalkArchive(archivePath, callback); err != nil {
		log.Fatal(err)
	}
	if !found {
		log.Fatalf("%s: %q not found in archive (upstream layout change?)", archivePath, binaryPath)
	}

	hashFile := fmt.Sprintf("%s.sha256.bin", archivePath)
	err = os.WriteFile(hashFile, hashResult[:], 0666)
	if err != nil {
		log.Fatal(err)
	}

	_, err = fmt.Fprintf(os.Stderr, "sha256 of %s/%s: %s\n", archivePath, binaryPath, hex.EncodeToString(hashResult[:]))
	if err != nil {
		log.Fatal(err)
	}
}
