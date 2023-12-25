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
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const (
	QodanaEndpoint     = "ENDPOINT"
	DefaultEndpoint    = "qodana.cloud"
	maxNumberOfRetries = 3
	waitTimeout        = time.Second * 30
	requestTimeout     = time.Second * 30
)

func getCloudBaseUrl() string {
	return fmt.Sprintf("https://%s", GetEnvWithDefault(QodanaEndpoint, DefaultEndpoint))
}

func getCloudApiBaseUrl() string {
	return fmt.Sprintf("https://api.%s", GetEnvWithDefault(QodanaEndpoint, DefaultEndpoint))
}

// GetCloudTeamsPageUrl returns the team page URL on Qodana Cloud
func GetCloudTeamsPageUrl(origin string, path string) string {
	name := filepath.Base(path)

	return strings.Join([]string{"https://", GetEnvWithDefault(QodanaEndpoint, DefaultEndpoint), "/?origin=", origin, "&name=", name}, "")
}

type QdClient struct {
	httpClient *http.Client
	token      string
}

func NewQdClient(token string) *QdClient {
	return &QdClient{
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
		token: token,
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

func (client *QdClient) ValidateToken() interface{} {
	result := client.getProject()
	switch v := result.(type) {
	case Success:
		return v.Data["name"]
	default:
		return ""
	}
}

func (client *QdClient) getProject() RequestResult {
	return client.doRequest("/v1/projects", "GET", nil, nil)
}

func (client *QdClient) doRequest(path, method string, headers map[string]string, body []byte) RequestResult {
	url := getCloudApiBaseUrl() + path
	var resp *http.Response
	var responseErr error

	for i := 0; i < maxNumberOfRetries; i++ {
		req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
		if err != nil {
			return RequestError{Err: err}
		}

		req.Header.Set("Authorization", "Bearer "+client.token)
		req.Header.Set("Content-Type", "application/json")
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		resp, responseErr = client.httpClient.Do(req)
		if responseErr == nil {
			break
		}
		time.Sleep(waitTimeout)
	}
	if responseErr != nil {
		return RequestError{Err: responseErr}
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(resp.Body)

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
