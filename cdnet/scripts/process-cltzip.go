// Compute SHA-256 checksum for clt.zip artifact.
package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"

	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
)

const DLL_PATH_PATTERN = `^tools/[^/]+/any/JetBrains\.CommandLine\.Products\.dll$`

var dllPathRegex = regexp.MustCompile(DLL_PATH_PATTERN)

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
			log.Fatalf("Found multiple matches for `%s` inside clt.zip: '%s', '%s'.", DLL_PATH_PATTERN, dllPath, path)
		}
		dllPath = path

		err := (error)(nil)
		dllHash, err = utils.GetSha256(stream)
		if err != nil {
			log.Fatalf("sha256 error: %s", err)
		}
	}

	stat, err := os.Stat("clt.zip")
	if err != nil {
		log.Fatal(err)
	}
	if stat.Size() == 0 {
		// Assume someone does not have clt.zip and just `touch`-ed it to proceed with the build.
		_, err = fmt.Fprintln(os.Stderr, "clt.zip is a 0-byte file, will generate mock hashsum.")
		if err != nil {
			log.Fatal(err)
		}
	} else {
		err = utils.WalkZipArchive("clt.zip", callback)
		if err != nil {
			log.Fatal(err)
		}
		if dllPath == "" {
			log.Fatalf("Could not find a file matching `%s` DLL inside clt.zip.", DLL_PATH_PATTERN)
		}
	}

	err = os.WriteFile("clt.sha256.bin", dllHash[:], 0666)
	if err != nil {
		log.Fatal(err)
	}

	_, err = fmt.Fprintf(os.Stderr, "sha256 of clt.zip/%s: %s\n", dllPath, hex.EncodeToString(dllHash[:]))
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile("clt.path.txt", []byte(dllPath), 0666)
	if err != nil {
		log.Fatal(err)
	}
}
