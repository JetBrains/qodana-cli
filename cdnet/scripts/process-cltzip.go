//go:build ignore

// Compute SHA-256 checksum for clt.zip artifact.
package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"

	"github.com/JetBrains/qodana-cli/internal/foundation/archive"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	"github.com/JetBrains/qodana-cli/internal/foundation/hash"
)

const DllPathPattern = `^tools/[^/]+/any/JetBrains\.CommandLine\.Products\.dll$`

var dllPathRegex = regexp.MustCompile(DllPathPattern)

func main() {
	// Find and hash commandline tools DLL inside the archive
	var dllPath = ""
	var dllHash [32]byte
	callback := func(path string, info os.FileInfo, stream io.Reader) {
		if info.IsDir() {
			return
		}
		if !dllPathRegex.MatchString(path) {
			return
		}

		if dllPath != "" {
			log.Fatalf("Found multiple matches for `%s` inside clt.zip: '%s', '%s'.", DllPathPattern, dllPath, path)
		}
		dllPath = path

		err := (error)(nil)
		dllHash, err = hash.GetSha256(stream)
		if err != nil {
			log.Fatalf("sha256 error: %s", err)
		}
	}

	// The download step (download-cdnet.go) guarantees a real archive or fails; a missing or empty
	// clt.zip here means it was skipped (no QODANA_CLI_DEPS_TOKEN) — fail loud rather than mock.
	stat, err := os.Stat("clt.zip")
	if err != nil {
		log.Fatalf("clt.zip: the download step must run with QODANA_CLI_DEPS_TOKEN set (%s)", err)
	}
	if stat.Size() == 0 {
		log.Fatal("clt.zip is empty; the download step must run with QODANA_CLI_DEPS_TOKEN set")
	}
	if err := archive.WalkZipArchive("clt.zip", callback); err != nil {
		log.Fatal(err)
	}
	if dllPath == "" {
		log.Fatalf("Could not find a file matching `%s` DLL inside clt.zip.", DllPathPattern)
	}

	err = fs.WriteFileAtomic("clt.sha256.bin", dllHash[:], 0666)
	if err != nil {
		log.Fatal(err)
	}

	_, err = fmt.Fprintf(os.Stderr, "sha256 of clt.zip/%s: %s\n", dllPath, hex.EncodeToString(dllHash[:]))
	if err != nil {
		log.Fatal(err)
	}

	err = fs.WriteFileAtomic("clt.path.txt", []byte(dllPath), 0666)
	if err != nil {
		log.Fatal(err)
	}
}
