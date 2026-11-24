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

package commoncontext

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type shownReport struct {
	target string
	err    error
}

// showReport runs ShowReport in the background and returns what its shown callback gets. It waits for ShowReport to
// return, or, for a local report served until the test ends, for the browser to open it.
func showReport(t *testing.T, resultsDir string, reportDir string, port int) shownReport {
	opened, done := make(chan string, 1), make(chan struct{})
	original := openBrowser
	openBrowser = func(url string) error { opened <- url; return nil }
	shown := make(chan shownReport, 1)
	go func() {
		defer close(done)
		ShowReport(
			t.TempDir(), resultsDir, reportDir, port,
			func(target string, err error) { shown <- shownReport{target, err} },
		)
	}()
	result := <-shown
	select {
	case <-done:
	case url := <-opened:
		assert.Equal(t, fmt.Sprintf("http://localhost:%d", port), url)
	}
	openBrowser = original
	return result
}

func freePort(t *testing.T) int {
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

func TestShowReportOfQodanaCloud(t *testing.T) {
	resultsDir := t.TempDir()
	url := `{"cloud": {"url": "http://localhost:1/report"}}`
	require.NoError(t, os.WriteFile(filepath.Join(resultsDir, "open-in-ide.json"), []byte(url), 0o644))

	assert.Equal(t, shownReport{"cloud", nil}, showReport(t, resultsDir, t.TempDir(), freePort(t)))
}

// TestShowLocalReport leaves the server running until the tests end: ShowReport has no way to stop it.
func TestShowLocalReport(t *testing.T) {
	reportDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(reportDir, "index.html"), []byte("report"), 0o644))
	port := freePort(t)

	assert.Equal(t, shownReport{"local", nil}, showReport(t, t.TempDir(), reportDir, port))
	response, err := http.Get(fmt.Sprintf("http://localhost:%d/index.html", port))
	require.NoError(t, err, "the report is served once it is shown")
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)
	assert.Equal(t, "report", string(body))
}

func TestShowMissingReport(t *testing.T) {
	logrus.StandardLogger().ExitFunc = func(int) {}
	t.Cleanup(func() { logrus.StandardLogger().ExitFunc = nil })

	shown := showReport(t, t.TempDir(), filepath.Join(t.TempDir(), "missing"), freePort(t))
	assert.Equal(t, "local", shown.target)
	assert.ErrorIs(t, shown.err, ErrReportMissing)
}

func TestShowReportOnAPortInUse(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	reportDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(reportDir, "index.html"), []byte("report"), 0o644))

	shown := showReport(t, t.TempDir(), reportDir, listener.Addr().(*net.TCPAddr).Port)
	assert.Equal(t, "local", shown.target)
	assert.Error(t, shown.err)
	assert.NotErrorIs(t, shown.err, ErrReportMissing)
}
