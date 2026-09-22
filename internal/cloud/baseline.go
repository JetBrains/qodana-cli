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
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/sarif"
	log "github.com/sirupsen/logrus"
)

const (
	qodanaBaselineUri = "/linters/baseline"
	// the SARIF report the baseline problems are wrapped in, with the name of the tool filled in
	baselineSarifHeader = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":%s}},"results":[`
	baselineSarifFooter = `]}]}`
)

// WriteBaseline writes the baseline stored in Qodana Cloud for the given tool to file as a SARIF report.
//
// Qodana Cloud streams the baseline problem by problem, so it is converted the same way: neither the
// response nor the report is ever held in memory as a whole.
func (client *QdClient) WriteBaseline(toolName string, file *os.File) (bool, error) {
	request := NewCloudRequest(qodanaBaselineUri + baselineQuery(toolName))

	written := false
	request.ReadResponse = func(body io.Reader) error {
		// a retried request writes the report anew, over what a failed attempt has left
		if err := file.Truncate(0); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		out := bufio.NewWriter(file)
		var err error
		if written, err = writeBaselineSarif(toolName, body, out); err != nil || !written {
			return err
		}
		return out.Flush()
	}

	if _, err := client.streaming().doRequest(&request); err != nil {
		var apiError *APIError
		if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
			// a Qodana Cloud which doesn't serve baselines at all
			log.Debugf("Qodana Cloud has no baseline endpoint: %v", err)
			return false, nil
		}
		return false, fmt.Errorf("baseline request failed: %w", err)
	}
	return written, nil
}

// streaming returns the client to download the baseline with.
func (client *QdClient) streaming() *QdClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = getRequestTimeout()
	transport.IdleConnTimeout = getRequestTimeout()
	return &QdClient{
		httpClient: &http.Client{Transport: transport},
		apiUrl:     client.apiUrl,
		token:      client.token,
	}
}

// baselineQuery asks for the baseline of one tool, which Qodana Cloud stores under the trimmed and
// lowercased tool name. No tool name asks for the baseline of every tool of the project.
func baselineQuery(toolName string) string {
	tool := strings.ToLower(strings.TrimSpace(toolName))
	if tool == "" {
		return ""
	}
	return "?" + url.Values{"toolName": []string{tool}}.Encode()
}

// writeBaselineSarif converts the streamed baseline response to a SARIF report of the given tool,
// copying one problem at a time. It reports whether the response had any problem to write.
func writeBaselineSarif(toolName string, in io.Reader, out io.Writer) (bool, error) {
	decoder := json.NewDecoder(in)
	hasBaseline, err := openBaselineArray(decoder)
	if err != nil {
		return false, fmt.Errorf("unexpected baseline response: %w", err)
	}
	if !hasBaseline {
		return false, nil
	}
	name, err := json.Marshal(toolName) // quoted and escaped for the report
	if err != nil {
		return false, err
	}

	encoder := json.NewEncoder(out)
	problems := 0
	for decoder.More() {
		var problem sarif.Result
		if err := decoder.Decode(&problem); err != nil {
			return false, fmt.Errorf("failed to read baseline problem #%d: %w", problems+1, err)
		}
		separator := ","
		if problems == 0 {
			separator = fmt.Sprintf(baselineSarifHeader, name)
		}
		if _, err := io.WriteString(out, separator); err != nil {
			return false, err
		}
		if err := encoder.Encode(&problem); err != nil {
			return false, fmt.Errorf("failed to write baseline problem #%d: %w", problems+1, err)
		}
		problems++
	}
	// the list of problems must be closed by the server, otherwise the response was cut short
	if _, err := decoder.Token(); err != nil {
		return false, fmt.Errorf("baseline response was truncated after %d problems: %w", problems, err)
	}
	if problems == 0 {
		return false, nil
	}
	_, err = io.WriteString(out, baselineSarifFooter)
	return err == nil, err
}

// openBaselineArray reads the response up to the first problem of the baseline, skipping any other
// field of the response. It reports whether the response has a baseline at all: the body of an
// enabled but empty baseline is empty.
func openBaselineArray(decoder *json.Decoder) (bool, error) {
	token, err := decoder.Token()
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !isDelimiter(token, '{') {
		return false, fmt.Errorf("expected an object, got '%v'", token)
	}

	for decoder.More() {
		field, err := decoder.Token()
		if err != nil {
			return false, err
		}
		if field != "baseline" {
			var skipped json.RawMessage
			if err := decoder.Decode(&skipped); err != nil {
				return false, err
			}
			continue
		}
		if token, err = decoder.Token(); err != nil {
			return false, err
		}
		if !isDelimiter(token, '[') {
			return false, fmt.Errorf("expected a list of problems, got '%v'", token)
		}
		return true, nil
	}
	return false, nil
}

func isDelimiter(token json.Token, delimiter rune) bool {
	value, ok := token.(json.Delim)
	return ok && value == json.Delim(delimiter)
}
