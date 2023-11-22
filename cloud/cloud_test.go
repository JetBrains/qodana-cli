/*
 * Copyright 2021-2023 JetBrains s.r.o.
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
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestGetProjectByBadToken(t *testing.T) {
	client := NewQdClient("https://www.jetbrains.com")
	result := client.getProject()
	switch v := result.(type) {
	case Success:
		t.Errorf("Did not expect request error: %v", v)
	case APIError:
		if v.StatusCode > http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, v.StatusCode)
		}
	case RequestError:
		t.Errorf("Did not expect request error: %v", v)
	default:
		t.Error("Unknown result type")
	}
}

func TestValidateToken(t *testing.T) {
	client := NewQdClient("kek")
	if projectName := client.ValidateToken(); projectName != "" {
		t.Errorf("Problem")
	}
}

func TestGetReportUrl(t *testing.T) {
	for _, tc := range []struct {
		name           string
		jsonData       jsonData
		reportUrlFile  string
		expectedReport string
	}{
		{
			name:           "valid json data and url",
			jsonData:       jsonData{Cloud: cloudInfo{URL: "https://cloud.qodana.com/report/url"}},
			reportUrlFile:  "https://raw.qodana.com/report/url",
			expectedReport: "https://cloud.qodana.com/report/url",
		},
		{
			name:           "invalid json data, valid url file data",
			jsonData:       jsonData{Cloud: cloudInfo{URL: ""}},
			reportUrlFile:  "https://raw.qodana.com/report/url",
			expectedReport: "https://raw.qodana.com/report/url",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			jsonFile := filepath.Join(dir, openInIdeJson)
			jsonFileData, _ := json.Marshal(tc.jsonData)
			if err := os.WriteFile(jsonFile, jsonFileData, 0644); err != nil {
				t.Fatal(err)
			}
			urlFile := filepath.Join(dir, legacyReportFile)
			if err := os.WriteFile(urlFile, []byte(tc.reportUrlFile), 0644); err != nil {
				t.Fatal(err)
			}

			actual := GetReportUrl(dir)
			if actual != tc.expectedReport {
				t.Fatalf("Expected \"%s\" but got \"%s\"", tc.expectedReport, actual)
			}
		})
	}
}
