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
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/JetBrains/qodana-cli/internal/sarif"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// baselineBody is a shortened response of the Qodana Cloud baseline endpoint.
const baselineBody = `{
  "baseline": [
    {
      "ruleId": "KDocUnresolvedReference",
      "level": "warning",
      "message": {"text": "Cannot resolve symbol 'month'"},
      "locations": [
        {
          "physicalLocation": {
            "artifactLocation": {"uri": "src/main/kotlin/Queries.kt", "uriBaseId": "SRCROOT"},
            "region": {"startLine": 382, "startColumn": 20, "charLength": 5}
          }
        }
      ],
      "partialFingerprints": {
        "equalIndicator/v1": "03c59a481847203d909a649a4b454b798fe4566acf09e2b91517e02696a80705",
        "equalIndicator/v2": "a16c4a7e90cf7b1d"
      },
      "properties": {"cloudId": "ba814994-e3e0-4930-bc95-2a1928b5687d"}
    },
    {
      "ruleId": "VulnerableLibrariesLocal",
      "level": "error",
      "message": {"text": "Dependency maven:org.postgresql:postgresql:42.3.8 is vulnerable"},
      "partialFingerprints": {"equalIndicator/v1": "4952380790a9f56b905175b616e86d974ee290c4d2188b47d0032dcb61f1e5fb"}
    }
  ]
}`

func writeBaselineToString(t *testing.T, toolName string, body string) (bool, string, error) {
	t.Helper()
	out := &strings.Builder{}
	written, err := writeBaselineSarif(toolName, strings.NewReader(body), out)
	return written, out.String(), err
}

// TestWriteBaselineSarif verifies that the streamed problems become a SARIF report of the tool the
// baseline was requested for, keeping the data baseline-cli matches the problems by.
func TestWriteBaselineSarif(t *testing.T) {
	written, result, err := writeBaselineToString(t, "QDJVM", baselineBody)
	require.NoError(t, err)
	require.True(t, written)

	var report sarif.Report
	require.NoError(t, json.Unmarshal([]byte(result), &report), "written report: %s", result)

	assert.Equal(t, "2.1.0", report.Version)
	require.Len(t, report.Runs, 1)
	assert.Equal(t, "QDJVM", report.Runs[0].Tool.Driver.Name)

	problems := report.Runs[0].Results
	require.Len(t, problems, 2)

	assert.Equal(t, "KDocUnresolvedReference", problems[0].RuleId)
	assert.Equal(t, "warning", problems[0].Level)
	assert.Equal(t, "Cannot resolve symbol 'month'", problems[0].Message.Text)
	assert.Equal(
		t,
		map[string]string{
			"equalIndicator/v1": "03c59a481847203d909a649a4b454b798fe4566acf09e2b91517e02696a80705",
			"equalIndicator/v2": "a16c4a7e90cf7b1d",
		},
		problems[0].PartialFingerprints,
	)
	require.Len(t, problems[0].Locations, 1)
	assert.Equal(t, "src/main/kotlin/Queries.kt", problems[0].Locations[0].PhysicalLocation.ArtifactLocation.Uri)
	assert.Equal(t, int64(382), problems[0].Locations[0].PhysicalLocation.Region.StartLine)
	assert.Equal(
		t,
		map[string]any{"cloudId": "ba814994-e3e0-4930-bc95-2a1928b5687d"},
		problems[0].Properties.AdditionalProperties,
	)

	assert.Equal(t, "VulnerableLibrariesLocal", problems[1].RuleId)
	assert.Equal(t, "error", problems[1].Level)
}

// TestWriteBaselineSarifFingerprints verifies that the fingerprints, which the baseline is matched
// by, reach the report under the name the SARIF standard defines. A Qodana Cloud old enough to
// spell them partialFingerPrints is read just as well.
func TestWriteBaselineSarifFingerprints(t *testing.T) {
	for _, spelling := range []string{"partialFingerprints", "partialFingerPrints"} {
		t.Run(
			spelling, func(t *testing.T) {
				body := fmt.Sprintf(
					`{"baseline":[{"ruleId":"Rule","message":{"text":"problem"},%q:{"equalIndicator/v1":"fingerprint"}}]}`,
					spelling,
				)

				written, result, err := writeBaselineToString(t, "QDJVM", body)
				require.NoError(t, err)
				require.True(t, written)

				assert.Contains(t, result, `"partialFingerprints":{"equalIndicator/v1":"fingerprint"}`)
				assert.NotContains(t, result, `"partialFingerPrints"`)
			},
		)
	}
}

func TestWriteBaselineSarifWithoutProblems(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"empty body", ""},
		{"blank body", "  \n"},
		{"empty object", "{}"},
		{"empty baseline", `{"baseline": []}`},
		{"empty baseline with other fields", `{"total": 0, "baseline": []}`},
	} {
		t.Run(
			tc.name, func(t *testing.T) {
				written, result, err := writeBaselineToString(t, "QDJVM", tc.body)
				require.NoError(t, err)
				assert.False(t, written)
				assert.Empty(t, result)
			},
		)
	}
}

// TestWriteBaselineSarifTruncated verifies that a response cut short by the server is an error: a
// shorter baseline would silently un-baseline known problems.
func TestWriteBaselineSarifTruncated(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"cut before the baseline", `{"baseline"`},
		{"cut inside a problem", baselineBody[:len(baselineBody)/2]},
		{"cut after a problem", baselineBody[:strings.Index(baselineBody, "VulnerableLibrariesLocal")-10]},
		{"unclosed list of problems", strings.TrimSuffix(strings.TrimSpace(baselineBody), "]\n}")},
	} {
		t.Run(
			tc.name, func(t *testing.T) {
				written, _, err := writeBaselineToString(t, "QDJVM", tc.body)
				assert.Error(t, err)
				assert.False(t, written)
			},
		)
	}
}

func TestWriteBaselineSarifUnexpectedResponse(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"not an object", `["problem"]`},
		{"baseline is not a list", `{"baseline": {}}`},
		{"problem is not an object", `{"baseline": ["problem"]}`},
	} {
		t.Run(
			tc.name, func(t *testing.T) {
				written, _, err := writeBaselineToString(t, "QDJVM", tc.body)
				assert.Error(t, err)
				assert.False(t, written)
			},
		)
	}
}

// requestBaseline serves the given handler and writes the baseline it responds with to a file, the
// way an analysis does. It returns whether a baseline was written and the contents of the file.
func requestBaseline(t *testing.T, handler http.HandlerFunc) (bool, string, error) {
	t.Helper()
	return requestBaselineOf(t, "QDJVM", handler)
}

func requestBaselineOf(t *testing.T, toolName string, handler http.HandlerFunc) (bool, string, error) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	path := filepath.Join(t.TempDir(), "baseline.sarif.json")
	file, err := os.Create(path)
	require.NoError(t, err)

	endpoints := QdApiEndpoints{RootEndpoint: &QdRootEndpoint{Url: server.URL}, LintersApiUrl: server.URL}
	written, requestErr := endpoints.NewLintersApiClient("token").WriteBaseline(toolName, file)
	require.NoError(t, file.Close())

	report, err := os.ReadFile(path)
	require.NoError(t, err)
	return written, string(report), requestErr
}

func TestWriteBaseline(t *testing.T) {
	var requestUri, authorization, acceptEncoding string
	written, report, err := requestBaseline(
		t, func(w http.ResponseWriter, r *http.Request) {
			requestUri = r.URL.RequestURI()
			authorization = r.Header.Get("Authorization")
			acceptEncoding = r.Header.Get("Accept-Encoding")
			_, _ = w.Write([]byte(baselineBody))
		},
	)

	require.NoError(t, err)
	require.True(t, written)
	assert.Equal(t, "/linters/baseline?toolName=qdjvm", requestUri)
	assert.Equal(t, "Bearer token", authorization)
	assert.Contains(t, acceptEncoding, "gzip", "a streamed baseline should be requested compressed")
	assert.Contains(t, report, `"name":"QDJVM"`)
	assert.Contains(t, report, "KDocUnresolvedReference")
}

// TestWriteBaselineOfAllTools verifies that a linter of an unknown product code asks for the whole
// baseline of the project: the problems of the other tools are never matched, unlike the ones of its
// own tool, which would be missing from a baseline of the wrong name.
func TestWriteBaselineOfAllTools(t *testing.T) {
	var requestUri string
	written, report, err := requestBaselineOf(
		t, "", func(w http.ResponseWriter, r *http.Request) {
			requestUri = r.URL.RequestURI()
			_, _ = w.Write([]byte(baselineBody))
		},
	)

	require.NoError(t, err)
	require.True(t, written)
	assert.Equal(t, "/linters/baseline", requestUri, "no tool name asks for the whole baseline")
	assert.Contains(t, report, `"name":""`)
	assert.Contains(t, report, "KDocUnresolvedReference")
}

// TestWriteBaselineGzipped verifies that the baseline is read from the compressed response the cloud
// sends to a client which accepts gzip.
func TestWriteBaselineGzipped(t *testing.T) {
	written, report, err := requestBaseline(
		t, func(w http.ResponseWriter, r *http.Request) {
			require.Contains(t, r.Header.Get("Accept-Encoding"), "gzip")
			w.Header().Set("Content-Encoding", "gzip")
			compressed := gzip.NewWriter(w)
			_, _ = compressed.Write([]byte(baselineBody))
			require.NoError(t, compressed.Close())
		},
	)

	require.NoError(t, err)
	require.True(t, written)
	assert.Contains(t, report, "VulnerableLibrariesLocal")
}

// TestWriteBaselineStreamsProblems verifies that a baseline of many problems is written in full.
func TestWriteBaselineStreamsProblems(t *testing.T) {
	const problems = 5000
	written, report, err := requestBaseline(
		t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"baseline":[`)
			for i := 0; i < problems; i++ {
				if i > 0 {
					_, _ = io.WriteString(w, ",")
				}
				_, _ = fmt.Fprintf(
					w,
					`{"ruleId":"Rule%d","message":{"text":"problem %d"},"partialFingerprints":{"equalIndicator/v1":"%d"}}`,
					i, i, i,
				)
			}
			_, _ = io.WriteString(w, `]}`)
		},
	)
	require.NoError(t, err)
	require.True(t, written)

	var parsed sarif.Report
	require.NoError(t, json.Unmarshal([]byte(report), &parsed))
	require.Len(t, parsed.Runs, 1)
	assert.Len(t, parsed.Runs[0].Results, problems)
	assert.Equal(t, "Rule4999", parsed.Runs[0].Results[problems-1].RuleId)
}

// TestWriteBaselineOutlastsRequestTimeout verifies that a download slower than the configured
// request timeout isn't cut off: the timeout covers reading the body, which a big baseline outlasts.
func TestWriteBaselineOutlastsRequestTimeout(t *testing.T) {
	t.Setenv(qdenv.QodanaCloudRequestTimeoutEnv, "1")
	written, report, err := requestBaseline(
		t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"baseline":[`)
			for i := 0; i < 3; i++ {
				if i > 0 {
					_, _ = io.WriteString(w, ",")
				}
				_, _ = fmt.Fprintf(w, `{"ruleId":"Rule%d","message":{"text":"problem %d"}}`, i, i)
				w.(http.Flusher).Flush()
				time.Sleep(500 * time.Millisecond) // the whole body takes longer than the timeout
			}
			_, _ = io.WriteString(w, `]}`)
		},
	)

	require.NoError(t, err)
	require.True(t, written)
	assert.Contains(t, report, "Rule2")
}

// TestWriteBaselineTruncatedGzip verifies that a compressed response cut short by the server is an
// error: gzip makes the body smaller, not the guarantees weaker.
func TestWriteBaselineTruncatedGzip(t *testing.T) {
	t.Setenv(qdenv.QodanaCloudRequestRetriesEnv, "1")
	compressed := &bytes.Buffer{}
	writer := gzip.NewWriter(compressed)
	_, err := writer.Write([]byte(baselineBody))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	half := compressed.Bytes()[:compressed.Len()/2]

	written, report, err := requestBaseline(
		t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(half)
		},
	)

	assert.Error(t, err)
	assert.False(t, written)
	assert.Empty(t, report)
}

func TestWriteBaselineDisabled(t *testing.T) {
	written, report, err := requestBaseline(
		t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	)

	require.NoError(t, err)
	assert.False(t, written)
	assert.Empty(t, report)
}

// TestWriteBaselineWithoutEndpoint verifies that a Qodana Cloud which doesn't serve baselines
// doesn't break the analysis.
func TestWriteBaselineWithoutEndpoint(t *testing.T) {
	written, report, err := requestBaseline(
		t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		},
	)

	require.NoError(t, err)
	assert.False(t, written)
	assert.Empty(t, report)
}

func TestWriteBaselineDeclinedToken(t *testing.T) {
	written, _, err := requestBaseline(
		t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	)

	assert.ErrorContains(t, err, "baseline request failed")
	assert.False(t, written)
}
