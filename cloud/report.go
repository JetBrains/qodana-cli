package cloud

import (
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

const legacyReportFile = "qodana.cloud"
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
		reportURL, err = readLegacyReportFile(resultsDir, legacyReportFile)
		if err != nil || reportURL == "" {
			log.Debugf("Unable to find the report url in %s", filepath.Join(resultsDir, legacyReportFile))
			return ""
		}
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

func readLegacyReportFile(resultsDir, fileName string) (string, error) {
	filePath := filepath.Join(resultsDir, fileName)
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	log.Debugf("Found report URL from (%s): %s", filePath, string(fileData))
	return string(fileData), nil
}

// SaveReportFile saves the report URL to the resultsDir/open-in-ide.json file if it does not exist.
func SaveReportFile(resultsDir, reportUrl string) {
	if reportUrl == "" {
		return
	}
	reportFilename := filepath.Join(resultsDir, openInIdeJson)
	if _, err := os.Stat(reportFilename); err != nil {
		var dataBytes []byte
		dataBytes, err = json.Marshal(jsonData{Cloud: cloudInfo{URL: reportUrl}})
		if err != nil {
			log.Errorf("Unable to marshal the report URL: %s", err)
			return
		}
		err = os.WriteFile(reportFilename, dataBytes, 0644)
		if err != nil {
			log.Errorf("Unable to save the report URL: %s", err)
			return
		}
	}
}
