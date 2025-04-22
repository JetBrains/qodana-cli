package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
)

func main() {
	if len(os.Args) <= 1 {
		log.Fatalf("Expected publisher-cli version as first argument")
	}
	version := os.Args[1]

	url := fmt.Sprintf("https://packages.jetbrains.team/maven/p/ij/intellij-dependencies/org/jetbrains/qodana/publisher-cli/%s/publisher-cli-%s.jar", version, version)
	utils.DownloadFile("publisher-cli.jar", url, nil)

	actualSha256Bytes, err := utils.GetFileSha256("publisher-cli.jar")
	if err != nil {
		log.Fatalf("Error while calculating SHA-256: %s", err)
	}
	actualSha256 := hex.EncodeToString(actualSha256Bytes[:])

	expectedSha256Url := url + ".sha256"
	resp, err := http.Get(expectedSha256Url)
	if err != nil {
		log.Fatalf("Failed to retrieve %s: %s", expectedSha256Url, err)
	}
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			log.Fatalf("Failed to close request body: %s", err)
		}
	}()

	expectedSha256Buffer := bytes.NewBuffer(nil)
	_, err = io.Copy(expectedSha256Buffer, resp.Body)
	if err != nil {
		log.Fatalf("Error while downloading %s: %s", expectedSha256Url, err)
	}
	expectedSha256 := expectedSha256Buffer.String()

	if actualSha256 != expectedSha256 {
		log.Fatalf(
			"Failed to verify SHA-256 hash sum for %s:\n"+
				"  expected: %s\n"+
				"    actual: %s",
			url, expectedSha256, actualSha256,
		)
	}
}
