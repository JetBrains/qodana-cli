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
	"bytes"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"net/url"
	"slices"
	"time"
)

const (
	QodanaEndpointEnv             = "QODANA_ENDPOINT"
	QodanaCloudRequestCooldownEnv = "QODANA_CLOUD_REQUEST_COOLDOWN"
	QodanaCloudRequestTimeoutEnv  = "QODANA_CLOUD_REQUEST_TIMEOUT"
	QodanaCloudRequestRetriesEnv  = "QODANA_CLOUD_REQUEST_RETRIES"

	DefaultEndpoint            = "qodana.cloud"
	defaultNumberOfRetries     = 3
	defaultCooldownTimeSeconds = 30
	defaultRequestTimeout      = 30
)

type QdRootEndpoint struct {
	Host string
}

type QdApiEndpoints struct {
	RootEndpoint  *QdRootEndpoint
	LintersApiUrl string
	CloudApiUrl   string
}

type QdClient struct {
	apiUrl     string
	httpClient *http.Client
	token      string
}

var endpoint *QdRootEndpoint
var endpointApis *QdApiEndpoints

func GetCloudApiEndpoints() *QdApiEndpoints {
	if endpointApis == nil {
		apis, err := GetCloudRootEndpoint().requestApiEndpoints()
		if err != nil {
			log.Fatalf("Failed to obtain proper API endpoints: %v", err)
		}
		endpointApis = apis
	}

	return endpointApis
}

func GetCloudRootEndpoint() *QdRootEndpoint {
	if endpoint != nil {
		return endpoint
	}
	userUrl := GetEnvWithDefault(QodanaEndpointEnv, DefaultEndpoint)
	host, err := parseRawURL(userUrl)
	if err != nil {
		log.Fatal(err)
	}
	endpoint = &QdRootEndpoint{host}
	return endpoint
}

func parseRawURL(rawUrl string) (host string, err error) {
	parsedUrl, err := url.ParseRequestURI(rawUrl)
	if err != nil || parsedUrl.Host == "" {
		parsedUrl, repErr := url.ParseRequestURI("https://" + rawUrl)
		if repErr != nil {
			return "", err
		}
		return parsedUrl.Host, nil
	}

	return parsedUrl.Host, nil
}

func (endpoints *QdApiEndpoints) NewCloudApiClient(token string) *QdClient {
	return &QdClient{
		httpClient: &http.Client{
			Timeout: getRequestTimeout(),
		},
		apiUrl: endpoints.CloudApiUrl,
		token:  token,
	}
}

func getRequestTimeout() time.Duration {
	return time.Duration(GetEnvWithDefaultInt(QodanaCloudRequestTimeoutEnv, defaultRequestTimeout)) * time.Second
}

func (endpoints *QdApiEndpoints) NewLintersApiClient(token string) *QdClient {
	return &QdClient{
		httpClient: &http.Client{
			Timeout: getRequestTimeout(),
		},
		apiUrl: endpoints.LintersApiUrl,
		token:  token,
	}
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("response code '%d', message '%s'", e.StatusCode, e.Message)
}

type QdCloudRequest struct {
	Path             string
	Method           string
	Headers          map[string]string
	Body             []byte
	AcceptedStatuses []int
	Retries          int
	Cooldown         int
}

func NewCloudRequest(path string) QdCloudRequest {
	return QdCloudRequest{
		Path:             path,
		Method:           "GET",
		AcceptedStatuses: []int{http.StatusUnauthorized, http.StatusNotFound},
		Retries:          GetEnvWithDefaultInt(QodanaCloudRequestRetriesEnv, defaultNumberOfRetries),
		Cooldown:         GetEnvWithDefaultInt(QodanaCloudRequestCooldownEnv, defaultCooldownTimeSeconds),
	}
}

func (client *QdClient) doRequest(request *QdCloudRequest) ([]byte, error) {
	var response []byte
	var err error
	for i := 1; i <= request.Retries; i++ {
		response, err = client.doRequestAttempt(request)
		if err == nil {
			return response, nil
		}
		var versionError *APIError
		if errors.As(err, &versionError) {
			if slices.Contains(request.AcceptedStatuses, versionError.StatusCode) {
				return nil, err // return if accepted status code, like 401
			}
		}
		log.Errorf("Attempt #%d of %d for request to '%s' failed. Error: %v", i, request.Retries, request.Path, err)
		if i < request.Retries {
			log.Printf("Next attempt in %d seconds", request.Cooldown)
			time.Sleep(time.Duration(request.Cooldown) * time.Second)
		}
	}

	return response, errors.New("failed to obtain proper cloud response")
}

func (client *QdClient) doRequestAttempt(request *QdCloudRequest) ([]byte, error) {
	requestUrl := client.apiUrl + request.Path
	var resp *http.Response
	var responseErr error

	req, err := http.NewRequest(request.Method, requestUrl, bytes.NewBuffer(request.Body))
	if err != nil {
		return nil, err
	}

	if client.token != "" {
		req.Header.Set("Authorization", "Bearer "+client.token)
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range request.Headers {
		req.Header.Set(key, value)
	}

	resp, responseErr = client.httpClient.Do(req)

	if responseErr != nil {
		return nil, responseErr
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(resp.Body)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return responseBody, nil
	}
	return nil, &APIError{StatusCode: resp.StatusCode, Message: string(responseBody)}
}
