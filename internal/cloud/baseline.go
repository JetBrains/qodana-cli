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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/JetBrains/qodana-cli/internal/sarif"
	log "github.com/sirupsen/logrus"
)

const (
	qodanaBaselineUri = "/linters/baseline"
	// the parts of the SARIF report the baseline problems are wrapped in, which the JSON encoder
	// fills in as the problems arrive:
	//
	//	{"version":"2.1.0","runs":[{"tool":{"driver":{"name":  "QDJVM"  }},"results":[  …problems…  ]}]}
	//	└────────────── header ─────────────────────────────┘ └ encoder ┘└── results ──┘└── footer ──┘
	baselineSarifHeader  = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":`
	baselineSarifResults = `}},"results":[`
	baselineSarifFooter  = `]}]}`
)

// ErrInvalidBaseline is the error of a baseline response which is not the list of problems it should be.
var ErrInvalidBaseline = errors.New("unexpected baseline response")

// BaselineStatus tells how Qodana Cloud has answered the request of a baseline.
type BaselineStatus int

const (
	// BaselineUnknown is no answer: the request failed or was never made.
	BaselineUnknown BaselineStatus = iota
	// BaselineWritten is a baseline with problems, written to the file.
	BaselineWritten
	// BaselineEmpty is a cloud baseline which is turned on but has no problems yet.
	BaselineEmpty
	// BaselineDisabled is a cloud baseline which is turned off for the project.
	BaselineDisabled
	// BaselineUnsupported is a Qodana Cloud which doesn't serve baselines at all.
	BaselineUnsupported
)

// NewBaselineClient returns the client of the linters API to download baselines with. A download has
// no overall deadline, because reading the baseline of a big project takes as long as it takes. A
// read which waits longer than the request timeout fails the download instead, as a read timeout of a
// socket does: the wait for the response, and each read of its body.
func (endpoints *QdApiEndpoints) NewBaselineClient(token string) *QdClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	timeout := getRequestTimeout()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, address)
		if err != nil || timeout <= 0 {
			return conn, err
		}
		return readTimeoutConn{Conn: conn, timeout: timeout}, nil
	}
	return &QdClient{
		httpClient: &http.Client{Transport: transport},
		apiUrl:     endpoints.LintersApiUrl,
		token:      token,
	}
}

// readTimeoutConn fails a read which waits longer than timeout. Without it a server which sends the
// headers and then goes quiet would hang the analysis for as long as it keeps the connection.
type readTimeoutConn struct {
	net.Conn
	timeout time.Duration
}

func (c readTimeoutConn) Read(p []byte) (int, error) {
	if err := c.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, err
	}
	return c.Conn.Read(p)
}

// WriteBaseline writes the baseline stored in Qodana Cloud for the given tool to the file at path as a
// SARIF report. An empty tool name asks for the baseline of every tool of the project. The status
// tells whether there was a baseline to write, and if there wasn't, why: the file holds the report
// only for BaselineWritten. The client must come from NewBaselineClient.
//
// Qodana Cloud streams the baseline problem by problem, so it is converted the same way: neither the
// response nor the report is ever held in memory as a whole.
func (client *QdClient) WriteBaseline(toolName string, path string) (BaselineStatus, error) {
	request := NewCloudRequest(qodanaBaselineUri + baselineQuery(toolName))
	// a new attempt does not change the answer for a token which may not read the baseline
	request.AcceptedStatuses = append(request.AcceptedStatuses, http.StatusForbidden)

	status := BaselineUnknown
	request.ReadResponse = func(statusCode int, body io.Reader) (err error) {
		status, err = writeBaselineFile(toolName, statusCode, body, path)
		return err
	}

	if _, err := client.doRequest(&request); err != nil {
		var apiError *APIError
		if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
			// a Qodana Cloud which doesn't serve baselines at all
			log.Debugf("Qodana Cloud has no baseline endpoint: %v", err)
			return BaselineUnsupported, nil
		}
		if request.lastError != nil && !errors.Is(err, request.lastError) {
			err = fmt.Errorf("%w: %w", err, request.lastError)
		}
		return BaselineUnknown, fmt.Errorf("baseline request failed: %w", err)
	}
	return status, nil
}

// writeBaselineFile writes a successful answer of Qodana Cloud to the baseline file. Each attempt of
// the request creates the file anew, over what a failed attempt has left.
func writeBaselineFile(toolName string, statusCode int, body io.Reader, path string) (status BaselineStatus, err error) {
	file, err := os.Create(path)
	if err != nil {
		return BaselineUnknown, err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()

	if statusCode == http.StatusNoContent {
		return BaselineDisabled, nil
	}
	out := bufio.NewWriter(file)
	written, err := writeBaselineSarif(toolName, body, out)
	if err != nil {
		return BaselineUnknown, err
	}
	if !written {
		return BaselineEmpty, nil
	}
	return BaselineWritten, out.Flush()
}

// baselineQuery asks for the baseline of one tool. The name is sent as the linters send it, which
// Qodana Cloud trims and lowercases itself. No tool name asks for the baseline of every tool.
func baselineQuery(toolName string) string {
	if strings.TrimSpace(toolName) == "" {
		return ""
	}
	return "?" + url.Values{"toolName": []string{toolName}}.Encode()
}

// writeBaselineSarif converts the streamed baseline response to a SARIF report of the given tool,
// copying one problem at a time. It reports whether the response had any problem to write.
func writeBaselineSarif(toolName string, in io.Reader, out io.Writer) (bool, error) {
	decoder := json.NewDecoder(in)
	if err := openBaselineArray(decoder); err != nil {
		return false, fmt.Errorf("%w: %w", ErrInvalidBaseline, err)
	}
	encoder := json.NewEncoder(out)
	problems := 0
	for decoder.More() {
		var problem sarif.Result
		if err := decoder.Decode(&problem); err != nil {
			if errors.As(err, new(*json.SyntaxError)) || errors.As(err, new(*json.UnmarshalTypeError)) {
				err = fmt.Errorf("%w: %w", ErrInvalidBaseline, err)
			}
			return false, fmt.Errorf("failed to read baseline problem #%d: %w", problems+1, err)
		}
		if problems == 0 {
			// the encoder writes the name quoted and escaped, so that no name can end the string
			// it is written in and change the report around it
			if _, err := io.WriteString(out, baselineSarifHeader); err != nil {
				return false, err
			}
			if err := encoder.Encode(toolName); err != nil {
				return false, err
			}
			if _, err := io.WriteString(out, baselineSarifResults); err != nil {
				return false, err
			}
		} else if _, err := io.WriteString(out, ","); err != nil {
			return false, err
		}
		if err := encoder.Encode(&problem); err != nil {
			return false, fmt.Errorf("failed to write baseline problem #%d: %w", problems+1, err)
		}
		problems++
	}
	// the list of problems must be closed by the server, otherwise the response was cut short
	if _, err := decoder.Token(); err != nil {
		return false, fmt.Errorf("%w: truncated after %d problems: %w", ErrInvalidBaseline, problems, err)
	}
	if problems == 0 {
		return false, nil
	}
	_, err := io.WriteString(out, baselineSarifFooter)
	return err == nil, err
}

// openBaselineArray reads the response up to the first problem of the baseline, skipping any other
// field of the response. Qodana Cloud always answers with a list of problems, an empty one for a
// baseline without problems, so a response without it is an error: it can only be cut short, and
// mistaking it for an empty baseline would hide that.
func openBaselineArray(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if errors.Is(err, io.EOF) {
		return errors.New("the response is empty")
	}
	if err != nil {
		return err
	}
	if !isDelimiter(token, '{') {
		return fmt.Errorf("expected an object, got '%v'", token)
	}

	for decoder.More() {
		field, err := decoder.Token()
		if err != nil {
			return err
		}
		if field != "baseline" {
			var skipped json.RawMessage
			if err := decoder.Decode(&skipped); err != nil {
				return err
			}
			continue
		}
		if token, err = decoder.Token(); err != nil {
			return err
		}
		if !isDelimiter(token, '[') {
			return fmt.Errorf("expected a list of problems, got '%v'", token)
		}
		return nil
	}
	// an object which isn't closed was cut off before the baseline
	if _, err := decoder.Token(); err != nil {
		return err
	}
	return errors.New("no baseline in the response")
}

func isDelimiter(token json.Token, delimiter rune) bool {
	value, ok := token.(json.Delim)
	return ok && value == json.Delim(delimiter)
}
