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
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestGetProjectByBadToken(t *testing.T) {
	apis := QdApiEndpoints{CloudApiUrl: "https://api.qodana.cloud/v1"}
	client := apis.NewCloudApiClient("bad_token")
	_, err := client.RequestProjectName()
	if err == nil {
		t.Errorf("Did not expect request success: %v", err)
	}
	var v *APIError
	switch {
	case errors.As(err, &v):
		if v.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status code %d, got %d. Message %s", http.StatusUnauthorized, v.StatusCode, v.Message)
		}
	default:
		t.Errorf("Unknown result type")
	}
}

// debug purpose only
func TestGetProjectByStaging(t *testing.T) {
	endpoint := QdRootEndpoint{Url: "https://cloud.sssa-stgn.aws.intellij.net"}
	token := os.Getenv("QODANA_TOKEN")
	if token == "" {
		t.Skip()
	}
	endpoints, err := endpoint.requestApiEndpoints()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	client := endpoints.NewCloudApiClient(token)
	_, err = client.RequestProjectName()
	var v *APIError
	switch {
	case errors.As(err, &v):
		if v.StatusCode > http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, v.StatusCode)
		}
	default:
		t.Error("Unknown result type")
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
			expectedReport: "https://cloud.qodana.com/report/url",
		},
		{
			name:           "invalid json data, valid url file data",
			jsonData:       jsonData{Cloud: cloudInfo{URL: ""}},
			expectedReport: "",
		},
	} {
		t.Run(
			tc.name, func(t *testing.T) {
				dir := t.TempDir()
				jsonFile := filepath.Join(dir, openInIdeJson)
				jsonFileData, _ := json.Marshal(tc.jsonData)
				if err := os.WriteFile(jsonFile, jsonFileData, 0644); err != nil {
					t.Fatal(err)
				}

				actual := GetReportUrl(dir)
				if actual != tc.expectedReport {
					t.Fatalf("Expected \"%s\" but got \"%s\"", tc.expectedReport, actual)
				}
			},
		)
	}
}
