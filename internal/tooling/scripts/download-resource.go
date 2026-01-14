//go:build ignore

// usage: go run scripts/download-resource.go [--force] <path>
// download-resource.go reads a config called "resources.json" in the working directory and downloads a file specified
// by <path> from the url specified in the config. When creating or updating entries, use --force to rewrite the hash
// sum automatically.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/platform/utils"
)

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
		actualSha256Bytes, err := utils.GetFileSha256(path)
		if err != nil {
			log.Fatalf("Error while calculating SHA-256: %s", err)
		}
		actualSha256 := hex.EncodeToString(actualSha256Bytes[:])

		if actualSha256 == resource.Sha256 {
			os.Exit(0)
		}
	}

	err = utils.DownloadFile(path, resolvedUrl, "", nil)
	if err != nil {
		log.Fatalf("Error while downloading %s: %s", resolvedUrl, err)
	}

	actualSha256Bytes, err := utils.GetFileSha256(path)
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
