//go:build ignore

/*
 * Copyright 2021-2024 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const JBR_VERSION_TAG = "v25.0.2b329.66.8"

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	ContentType        string `json:"content_type"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func main() {
	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken == "" {
		log.Fatalf("GITHUB_TOKEN is not set (required to access GitHub API for qodana-jbr repo)")
	}

	rel := parseGitHubReleaseForTag(JBR_VERSION_TAG)
	downloadJBRAssets(rel.Assets, ghToken)
}

func parseGitHubReleaseForTag(releaseTag string) Release {
	assetsList := "https://api.github.com/repos/JetBrains/qodana-jbr/releases/tags/" + releaseTag
	req, err := http.NewRequest(http.MethodGet, assetsList, nil)
	if err != nil {
		log.Fatalf("Error creating request %s: %v", assetsList, err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatalf("GITHUB_TOKEN is not set (required to access GitHub API for this repo/release)")
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("Error downloading %s: %v", assetsList, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("GitHub API error %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		log.Fatalf("Error decoding JSON from %s: %v", assetsList, err)
	}

	if len(rel.Assets) == 0 {
		log.Fatalf(
			"Release %q has no assets (tag=%q). Check that the release is published and assets are uploaded.",
			rel.TagName,
			releaseTag,
		)
	}

	fmt.Printf("\nFetched qodana-jbr assets for release tag %q:\n", rel.TagName)
	for _, a := range rel.Assets {
		fmt.Printf(" - %s (%d bytes)\n", a.Name, a.Size)
	}
	return rel
}

func downloadJBRAssets(assets []Asset, ghToken string) {
	client := &http.Client{Timeout: 30 * time.Second}
	baseDir := "qodana-jbrs"

	for _, a := range assets {
		goos, goarch := detectOsArchTargetForAsset(a.Name)

		targetDir := filepath.Join(baseDir, fmt.Sprintf("%s-%s", goos, goarch))
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			log.Fatalf("Failed to create directory %s: %v", targetDir, err)
		}

		entries, err := os.ReadDir(targetDir)
		if err != nil {
			log.Fatalf("Failed to list directory %s: %v", targetDir, err)
		}

		dstPath := filepath.Join(targetDir, a.Name)
		if len(entries) == 1 && !entries[0].IsDir() && entries[0].Name() == a.Name {
			if fi, statErr := os.Stat(dstPath); statErr == nil && fi.Size() == a.Size {
				fmt.Printf("OK, SKIPPED: %s\n", dstPath)
				continue
			}
		}

		if len(entries) > 0 {
			for _, e := range entries {
				p := filepath.Join(targetDir, e.Name())
				fmt.Printf("UPDATING: %s -> %s\n", e.Name(), a.Name)
				if rmErr := os.RemoveAll(p); rmErr != nil {
					log.Fatalf("Failed to clear %s (removing %s): %v", targetDir, p, rmErr)
				}
			}
		}

		fmt.Printf("DOWNLOADING: %s\n", a.Name)
		downloadReleaseAssetByID(client, a.ID, dstPath, ghToken)
	}
}

func downloadReleaseAssetByID(
	client *http.Client,
	assetID int64,
	destPath, githubToken string,
) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/JetBrains/qodana-jbr/releases/assets/%d", assetID)

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		log.Fatalf("Error creating request %s: %v", apiURL, err)
	}

	req.Header.Set("Authorization", "Bearer "+githubToken)
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error downloading %s: %v", apiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("Couldn't fetch asset %d, http %d: %s", assetID, resp.StatusCode, resp.Status)
	}

	tmp := destPath + ".part"
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		log.Fatalf("Failed to create directory %s: %v", filepath.Dir(destPath), err)
	}

	out, err := os.Create(tmp)
	if err != nil {
		log.Fatalf("Failed to create file %s: %v", tmp, err)
	}

	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		log.Fatalf("Failed to download asset %d: %v", assetID, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		log.Fatalf("Failed to close file %s: %v", tmp, closeErr)
	}

	if err := os.Rename(tmp, destPath); err != nil {
		log.Fatalf("Failed to rename %s to %s: %v", tmp, destPath, err)
	}
}

var osArchPattern = regexp.MustCompile(`-(osx|linux|windows)-(x64|aarch64)-`)

func detectOsArchTargetForAsset(name string) (goos string, goarch string) {
	matches := osArchPattern.FindStringSubmatch(name)
	if matches == nil {
		log.Fatalf("Unsupported OS/arch in asset name: %s", name)
	}

	switch matches[1] {
	case "osx":
		goos = "darwin"
	case "linux":
		goos = "linux"
	case "windows":
		goos = "windows"
	}

	switch matches[2] {
	case "x64":
		goarch = "amd64"
	case "aarch64":
		goarch = "arm64"
	}

	return goos, goarch
}
