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
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
)

func TestRequestProjectToken(t *testing.T) {
	const orgToken = "org-token"
	const projectToken = "project-token"
	t.Setenv(qdenv.QodanaCloudRequestCooldownEnv, "0")

	for _, testData := range []struct {
		name          string
		status        int
		body          string
		expectedToken string
		declined      bool
		success       bool
	}{
		{
			name:          "existing project",
			status:        http.StatusOK,
			body:          fmt.Sprintf(`{"projectToken":"%s","expiresAt":"2026-09-23T12:00:00Z"}`, projectToken),
			expectedToken: projectToken,
			success:       true,
		},
		{
			name:          "created project",
			status:        http.StatusCreated,
			body:          fmt.Sprintf(`{"projectToken":"%s"}`, projectToken),
			expectedToken: projectToken,
			success:       true,
		},
		{name: "bad request", status: http.StatusBadRequest, body: `{"name":"validation_failed","details":"x"}`, declined: true},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"name":"invalid_token","details":"x"}`, declined: true},
		{name: "forbidden", status: http.StatusForbidden, body: `{"name":"no_permission","details":"x"}`, declined: true},
		{name: "empty token", status: http.StatusOK, body: `{"projectToken":""}`},
		{name: "malformed response", status: http.StatusOK, body: projectToken},
	} {
		t.Run(
			testData.name, func(t *testing.T) {
				svr := httptest.NewServer(
					http.HandlerFunc(
						func(w http.ResponseWriter, r *http.Request) {
							if r.URL.Path != publicProjectsPath {
								t.Errorf("expected path '%s', got '%s'", publicProjectsPath, r.URL.Path)
							}
							if r.Method != http.MethodPost {
								t.Errorf("expected POST, got %s", r.Method)
							}
							if auth := r.Header.Get("Authorization"); auth != "Bearer "+orgToken {
								t.Errorf("unexpected Authorization header '%s'", auth)
							}
							var req map[string]any
							if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
								t.Errorf("failed to decode request: %v", err)
							}
							expected := map[string]any{"projectQualifiedSlug": "my team:my-project", "tokenTtlSeconds": float64(3600)}
							if !reflect.DeepEqual(req, expected) {
								t.Errorf("expected request %v, got %v", expected, req)
							}
							w.WriteHeader(testData.status)
							_, _ = fmt.Fprint(w, testData.body)
						},
					),
				)
				defer svr.Close()

				apis := QdApiEndpoints{CloudApiUrl: svr.URL}
				token, err := apis.NewPublicApiClient(orgToken).RequestProjectToken("my team:my-project", time.Hour)
				if testData.success {
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
					if token != testData.expectedToken {
						t.Errorf("expected token '%s', got '%s'", testData.expectedToken, token)
					}
					return
				}
				if err == nil {
					t.Fatalf("expected an error, got token '%s'", token)
				}
				if errors.Is(err, ErrOrgTokenDeclined) != testData.declined {
					t.Errorf("unexpected declined state for error: %v", err)
				}
				if strings.Contains(err.Error(), orgToken) || strings.Contains(err.Error(), projectToken) {
					t.Errorf("error must not contain tokens: %v", err)
				}
			},
		)
	}
}
