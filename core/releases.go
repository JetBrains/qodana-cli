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

/*
* This file contains the code for sending the report to Qodana Cloud.
* The publisher is a part of Qodana linters.
* This will be refactored/removed after the proper endpoint is implemented.
 */

package core

import (
	"encoding/json"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	startup "github.com/JetBrains/qodana-cli/v2024/preparehost"
	"io"
	"os"
	"path/filepath"
)

func getProductFeed() string {
	if feed := os.Getenv("QD_PRODUCT_INTERNAL_FEED"); feed != "" {
		return feed
	}
	return "https://raw.githubusercontent.com/JetBrains/qodana-docker/main/feed/releases.json"
}

type Product struct {
	Code     string
	Releases []ReleaseInfo
}

type ReleaseInfo struct {
	Date                 string
	Type                 string
	Downloads            *map[string]ReleaseDownloadInfo
	Version              *string
	MajorVersion         *string
	Build                *string
	PrintableReleaseType *string
}

type ReleaseDownloadInfo struct {
	Link         string `json:"Link"`
	Size         uint64 `json:"Size,omitempty"`
	ChecksumLink string `json:"ChecksumLink"`
}

func GetProductByCode(code string) (*Product, error) {
	tempDir, err := os.MkdirTemp("", "productByCode")
	if err != nil {
		platform.ErrorMessage("Cannot create temp dir", err)
		return nil, err
	}

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			platform.ErrorMessage("Cannot clean up temp dir", err)
		}
	}(tempDir) // clean up

	path := filepath.Join(tempDir, "productInfo.json")

	if err := platform.DownloadFile(path, getProductFeed(), nil); err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			platform.ErrorMessage("Cannot read downloaded file", err)
		}
	}(file)

	byteValue, _ := io.ReadAll(file)

	var products []Product
	if err := json.Unmarshal(byteValue, &products); err != nil {
		return nil, err
	}

	for _, product := range products {
		if product.Code == code {
			return &product, nil
		}
	}

	return nil, nil
}

func SelectLatestCompatibleRelease(product *Product, reqType string) *ReleaseInfo {
	var latestRelease *ReleaseInfo
	latestDate := ""

	for i := 0; i < len(product.Releases); i++ {
		release := &product.Releases[i]
		if *release.MajorVersion == startup.VersionsMap[reqType] && release.Type == reqType && (latestRelease == nil || release.Date > latestDate) {
			latestRelease = release
			latestDate = release.Date
		}
	}

	return latestRelease
}
