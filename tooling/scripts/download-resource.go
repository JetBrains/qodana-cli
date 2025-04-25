// usage: go run scripts/download-resource.go [--force] <path>
// download-resource.go reads a config called "resources.json" in the working directory and downloads a file specified
// by <path> from the url specified in the config. When creating or updating entries, use --force to rewrite the hash
// sum automatically.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// GetSha256 computes a hash sum for a byte stream.
func GetSha256(stream io.Reader) (result [32]byte, err error) {
	hasher := sha256.New()
	_, err = io.Copy(hasher, stream)
	if err != nil {
		return result, err
	}

	copy(result[:], hasher.Sum(nil))
	return result, nil
}

// GetFileSha256 computes a hash sum from an existing file.
func GetFileSha256(path string) (result [32]byte, err error) {
	reader, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer func() {
		err = errors.Join(err, reader.Close())
	}()

	return GetSha256(reader)
}

// DownloadFile downloads a file from a given URL to a given filepath.
func DownloadFile(filepath string, url string) error {
	response, err := http.Head(url)
	if err != nil {
		return fmt.Errorf("error making HEAD request: %w", err)
	}

	sizeStr := response.Header.Get("Content-Length")
	if sizeStr == "" {
		sizeStr = "-1"
	}
	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		return fmt.Errorf("error converting Content-Length to integer: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("error making GET request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			fmt.Printf("Error while closing HTTP stream: %v\n", err)
		}
	}(resp.Body)

	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	defer func(out *os.File) {
		if err := out.Close(); err != nil {
			fmt.Printf("Error while closing output file: %v\n", err)
		}
	}(out)

	buffer := make([]byte, 1024)
	total := 0
	for {
		length, err := resp.Body.Read(buffer)
		if err != nil && err != io.EOF {
			return fmt.Errorf("error reading response body: %w", err)
		}
		total += length
		if length == 0 {
			break
		}
		if _, err = out.Write(buffer[:length]); err != nil {
			return fmt.Errorf("error writing to file: %w", err)
		}
	}

	// Check if the size matches, but only if the Content-Length header was present and valid
	if size > 0 && total != size {
		return fmt.Errorf("downloaded file size doesn't match expected size, got %d, expected %d", total, size)
	}

	return nil
}

type Resource struct {
	Url     string `json:"url"`
	Version string `json:"version"`
	Sha256  string `json:"sha256"`
}

func main() {
	forcePtr := flag.Bool("force", false, "Force download and update hash-sum to match new value.")
	flag.Parse()

	force := *forcePtr
	path := flag.Arg(0)
	if path == "" {
		log.Fatalf("Expected path as first argument")
	}
	if len(flag.Args()) > 1 {
		log.Fatalf("Unrecognized arguments: %v", flag.Args()[1:])
	}

	resourcesData, err := os.ReadFile("resources.json")
	if err != nil {
		log.Fatalf("Error while reading \"resources.json\": %s", err)
	}

	var resources map[string]Resource
	err = json.Unmarshal(resourcesData, &resources)
	if err != nil {
		log.Fatalf("Error while parsing json in \"resources.json\": %s", err)
	}

	resource, resourceExists := resources[path]
	if !resourceExists {
		log.Fatalf("Could not find an entry with key \"%s\" in \"resources.json\"", path)
	}

	resolvedUrl := strings.ReplaceAll(resource.Url, "$version", resource.Version)
	_, err = os.Stat(path)
	if err == nil && !force {
		// If a file is already downloaded, check its hash sum and exit early
		actualSha256Bytes, err := GetFileSha256(path)
		if err != nil {
			log.Fatalf("Error while calculating SHA-256: %s", err)
		}
		actualSha256 := hex.EncodeToString(actualSha256Bytes[:])

		if actualSha256 == resource.Sha256 {
			os.Exit(0)
		}
	}

	err = DownloadFile(path, resolvedUrl)
	if err != nil {
		log.Fatalf("Error while downloading %s: %s", resolvedUrl, err)
	}

	actualSha256Bytes, err := GetFileSha256(path)
	if err != nil {
		log.Fatalf("Error while calculating SHA-256: %s", err)
	}
	actualSha256 := hex.EncodeToString(actualSha256Bytes[:])

	if !force {
		if actualSha256 != resource.Sha256 {
			log.Fatalf(
				"Failed to verify SHA-256 hash sum for %s:\n"+
					"  expected: %s\n"+
					"    actual: %s",
				resolvedUrl, resource.Sha256, actualSha256,
			)
		}
	} else {
		resource.Sha256 = actualSha256
		resources[path] = resource
		resourcesData, err = json.MarshalIndent(resources, "", "    ")
		if err != nil {
			log.Fatalf("Error while serializing data for \"resources.json\": %s", err)
		}
		err = os.WriteFile("resources.json", resourcesData, 0644)
		if err != nil {
			log.Fatalf("Error while writing \"resources.json\": %s", err)
		}
	}
}
