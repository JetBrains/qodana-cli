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

func TestGetCloudTeamsPageUrl(t *testing.T) {
	endpoint := QdRootEndpoint{Url: "https://qodana.cloud"}
	result := endpoint.GetCloudTeamsPageUrl("github", "/path/to/myproject")
	expected := "https://qodana.cloud/?origin=github&name=myproject"
	if result != expected {
		t.Errorf("GetCloudTeamsPageUrl() = %q, want %q", result, expected)
	}
}

func TestParseProjectName(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    string
		wantErr bool
	}{
		{"valid json", `{"name": "my-project"}`, "my-project", false},
		{"empty name", `{"name": ""}`, "", false},
		{"invalid json", `{invalid}`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseProjectName([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("parseProjectName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseProjectName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApiVersionMismatchError(t *testing.T) {
	err := &ApiVersionMismatchError{
		ApiKind:           "Cloud",
		SupportedVersions: []string{"1.0", "1.1"},
	}
	msg := err.Error()
	if msg == "" {
		t.Error("Expected non-empty error message")
	}
	if len(msg) < 10 {
		t.Errorf("Error message too short: %s", msg)
	}
}

func TestToCloudVersion(t *testing.T) {
	tests := []struct {
		version     string
		wantMajor   int
		wantMinor   int
		wantErr     bool
	}{
		{"1.0", 1, 0, false},
		{"2.5", 2, 5, false},
		{"invalid", 0, 0, true},
		{"1.2.3", 0, 0, true},
		{"a.b", 0, 0, true},
		{"1.b", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got, err := ToCloudVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToCloudVersion(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
				return
			}
			if !tt.wantErr && (got.Major != tt.wantMajor || got.Minor != tt.wantMinor) {
				t.Errorf("ToCloudVersion(%q) = %v, want {%d, %d}", tt.version, got, tt.wantMajor, tt.wantMinor)
			}
		})
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

func TestGetEnvWithDefault(t *testing.T) {
	tests := []struct {
		name         string
		env          string
		value        string
		setEnv       bool
		defaultValue string
		expected     string
	}{
		{"env set", "TEST_ENV_SET", "value", true, "default", "value"},
		{"env not set", "TEST_ENV_NOT_SET", "", false, "default", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				_ = os.Setenv(tt.env, tt.value)
				defer func() {
					_ = os.Unsetenv(tt.env)
				}()
			}
			if got := GetEnvWithDefault(tt.env, tt.defaultValue); got != tt.expected {
				t.Errorf("GetEnvWithDefault() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetEnvWithDefaultInt(t *testing.T) {
	t.Run("env set", func(t *testing.T) {
		_ = os.Setenv("TEST_INT_ENV", "42")
		defer func() {
			_ = os.Unsetenv("TEST_INT_ENV")
		}()
		if got := GetEnvWithDefaultInt("TEST_INT_ENV", 10); got != 42 {
			t.Errorf("GetEnvWithDefaultInt() = %v, want 42", got)
		}
	})

	t.Run("env not set", func(t *testing.T) {
		if got := GetEnvWithDefaultInt("TEST_INT_NOT_SET", 99); got != 99 {
			t.Errorf("GetEnvWithDefaultInt() = %v, want 99", got)
		}
	})
}

func TestParseRawURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://example.com", false},
		{"valid http", "http://example.com", false},
		{"without scheme", "example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRawURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRawURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractVersions(t *testing.T) {
	descriptions := []ApiVersionDescription{
		{Version: "1.0", URL: "http://example.com/v1"},
		{Version: "2.0", URL: "http://example.com/v2"},
	}
	versions := extractVersions(descriptions)
	if len(versions) != 2 {
		t.Errorf("extractVersions() returned %d versions, want 2", len(versions))
	}
	if versions[0] != "1.0" || versions[1] != "2.0" {
		t.Errorf("extractVersions() returned incorrect versions: %v", versions)
	}
}
