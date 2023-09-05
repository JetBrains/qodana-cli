package core

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

const (
	baseUrl            = "https://api.qodana.cloud"
	maxNumberOfRetries = 3
	requestTimeout     = 3
)

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

		time.Sleep(30 * time.Second)
	}
	if err != nil {
		return RequestError{Err: err}
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var data map[string]interface{}
		if err := json.Unmarshal(responseBody, &data); err != nil {
			return RequestError{Err: err}
		}
		return Success{Data: data}
	}
	return APIError{StatusCode: resp.StatusCode, Message: string(responseBody)}
}
