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

package cloud

import (
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

const openInIdeJson = "open-in-ide.json"

type cloudInfo struct {
	URL string `json:"url"`
}
type jsonData struct {
	Cloud cloudInfo `json:"cloud"`
}

// GetReportUrl retrieves the Qodana Cloud report URL from the qodana.sarif.json in the specified results directory.
func GetReportUrl(resultsDir string) string {
	reportURL, err := readOpenInIde(resultsDir, openInIdeJson)
	if err != nil || reportURL == "" {
		log.Debugf("Unable to find the report url in %s", filepath.Join(resultsDir, openInIdeJson))
		return ""
	}
	return reportURL
}

func readOpenInIde(resultsDir, fileName string) (string, error) {
	filePath := filepath.Join(resultsDir, fileName)
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	data := jsonData{}
	err = json.Unmarshal(fileData, &data)
	if err != nil || data.Cloud.URL == "" {
		return "", err
	}

	log.Debugf("Found report URL from (%s): %s", filePath, data.Cloud.URL)
	return data.Cloud.URL, nil
}
