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
	"bytes"
	"encoding/json"
	"github.com/zalando/go-keyring"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultEndpoint    = "qodana.cloud"
	defaultService     = "qodana-cli"
	baseUrl            = "https://api.qodana.cloud"
	maxNumberOfRetries = 3
	waitTimeout        = time.Second * 30
	requestTimeout     = time.Second * 30
)

// GetCloudTeamsPageUrl returns the team page URL on Qodana Cloud
func GetCloudTeamsPageUrl(origin string, path string) string {
	name := filepath.Base(path)

	return strings.Join([]string{"https://", DefaultEndpoint, "/?origin=", origin, "&name=", name}, "")
}

// SaveCloudToken saves token to the system keyring
func SaveCloudToken(id string, token string) error {
	err := keyring.Set(defaultService, id, token)
	if err != nil {
		return err
	}
	return nil
}

// GetCloudToken returns token from the system keyring
func GetCloudToken(id string) (string, error) {
	secret, err := keyring.Get(defaultService, id)
	if err != nil {
		return "", err
	}
	return secret, nil
}

type QodanaClient struct {
	httpClient *http.Client
}

func NewQodanaClient() *QodanaClient {
	return &QodanaClient{
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}
}

type Success struct {
	Data map[string]interface{}
}

type RequestResult interface {
	isRequestResult()
}

type APIError struct {
	StatusCode int
	Message    string
}

type RequestError struct {
	Err error
}

func (Success) isRequestResult()      {}
func (APIError) isRequestResult()     {}
func (RequestError) isRequestResult() {}

func (client *QodanaClient) ValidateToken(token string) interface{} {
	result := client.GetProjectByToken(token)
	switch v := result.(type) {
	case Success:
		return v.Data["name"]
	default:
		return ""
	}
}

func (client *QodanaClient) GetProjectByToken(token string) RequestResult {
	return client.doRequest("/v1/projects", token, "GET", nil, nil)
}

func (client *QodanaClient) doRequest(path, token, method string, headers map[string]string, body []byte) RequestResult {
	url := baseUrl + path
	var resp *http.Response
	var err error

	for i := 0; i < maxNumberOfRetries; i++ {
		req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
		if err != nil {
			return RequestError{Err: err}
		}

		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		resp, err = client.httpClient.Do(req)
		if err == nil {
			break
		}
		time.Sleep(waitTimeout)
	}
	if err != nil {
		return RequestError{Err: err}
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		var data map[string]interface{}
		if err := json.Unmarshal(responseBody, &data); err != nil {
			return RequestError{Err: err}
		}
		return Success{Data: data}
	}
	return APIError{StatusCode: resp.StatusCode, Message: string(responseBody)}
}
