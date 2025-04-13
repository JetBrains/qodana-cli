// Compute SHA-256 checksum for clt.zip artifact.
package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
)

func main() {
	if len(os.Args) <= 1 {
		log.Fatalf("Expected path of the file to hash as argv[1] (relative from clt.zip root)")
	}
	targetPath := os.Args[1]

	// Compute hash for the clang-tidy binary
	hash := ([]byte)(nil)
	callback := func(path string, info os.FileInfo, stream io.Reader) {
		if info.IsDir() {
			return
		}
		if path != targetPath {
			return
		}

		err := (error)(nil)
		hash, err = utils.GetSha256(stream)
		if err != nil {
			log.Fatalf("sha256 error: %s", err)
		}
	}

	utils.WalkZipArchive("clt.zip", callback)
	err := os.WriteFile("clt.sha256.bin", hash, 0666)
	if err != nil {
		log.Fatal(err)
	}

	_, err = fmt.Fprintf(os.Stderr, "sha256 of the contents of clt.zip/%q: %s\n", targetPath, hex.EncodeToString(hash))
	if err != nil {
		log.Fatal(err)
	}
}
