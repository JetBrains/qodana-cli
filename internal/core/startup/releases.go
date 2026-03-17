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

package startup

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
)

func getProductFeedURL(linterName string) string {
	if feed := os.Getenv("QD_PRODUCT_INTERNAL_FEED"); feed != "" {
		return feed
	}
	return fmt.Sprintf(
		"https://download.jetbrains.com/qodana/feed/%s.releases.json",
		linterName,
	)
}

func getInternalAuth() string {
	if auth := os.Getenv("QD_PRODUCT_INTERNAL_AUTH"); auth != "" {
		return auth
	}
	return ""
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

func getProductByLinterName(linterName string) (*Product, error) {
	tempDir, err := os.MkdirTemp("", "productByLinterName")
	if err != nil {
		msg.ErrorMessage("Cannot create temp dir", err)
		return nil, err
	}

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			msg.ErrorMessage("Cannot clean up temp dir", err)
		}
	}(tempDir)

	url := getProductFeedURL(linterName)

	// Check if the product feed exists (404 will happen if native mode is not available for the chosen linter)
	resp, err := http.Head(url)
	if err == nil {
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				msg.ErrorMessage("Cannot close response body", err)
			}
		}(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
	}

	path := filepath.Join(tempDir, "productInfo.json")
	if err := utils.DownloadFile(path, url, "", nil); err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			msg.ErrorMessage("Cannot read downloaded file", err)
		}
	}(file)

	byteValue, _ := io.ReadAll(file)

	var prod Product
	if err := json.Unmarshal(byteValue, &prod); err != nil {
		return nil, err
	}

	return &prod, nil
}

func selectLatestCompatibleRelease(prod *Product, reqType string) *ReleaseInfo {
	var latestRelease *ReleaseInfo
	latestDate := ""

	for i := 0; i < len(prod.Releases); i++ {
		release := &prod.Releases[i]
		if *release.MajorVersion == product.ReleaseVersion && release.Type == reqType && (latestRelease == nil || release.Date > latestDate) {
			latestRelease = release
			latestDate = release.Date
		}
	}

	return latestRelease
}
