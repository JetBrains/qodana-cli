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

package platform

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/cloud"
	"github.com/JetBrains/qodana-cli/internal/sarif"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// baselineDownloaderStub writes the baseline it is given, the way the cloud client does. Without a
// baseline it answers with the status it is given.
type baselineDownloaderStub struct {
	toolName string
	baseline *sarif.Report
	status   cloud.BaselineStatus
	err      error
	asked    bool
}

func (s *baselineDownloaderStub) WriteBaseline(toolName string, path string) (cloud.BaselineStatus, error) {
	s.asked = true
	s.toolName = toolName
	if s.err != nil {
		return cloud.BaselineUnknown, s.err
	}
	if s.baseline == nil {
		return s.status, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return cloud.BaselineUnknown, err
	}
	defer func() { _ = file.Close() }()
	return cloud.BaselineWritten, json.NewEncoder(file).Encode(s.baseline)
}

func (s *baselineDownloaderStub) newClient() baselineDownloader { return s }

func reportWithToolName(toolName string) *sarif.Report {
	return &sarif.Report{
		Version: "2.1.0",
		Runs: []sarif.Run{
			{
				Tool:    &sarif.Tool{Driver: &sarif.ToolComponent{Name: toolName}},
				Results: []sarif.Result{},
			},
		},
	}
}

func TestDownloadCloudBaseline(t *testing.T) {
	// the system temp dir is small and sometimes backed by memory, the baseline can be huge
	t.Run("baseline is stored in the cache dir", func(t *testing.T) {
		client := &baselineDownloaderStub{baseline: reportWithToolName("QDNET")}
		cacheDir := t.TempDir()

		downloaded, err := downloadCloudBaseline(client, "QDNET", cacheDir)
		require.NoError(t, err)
		defer downloaded.Cleanup()

		require.IsType(t, CloudBaseline{}, downloaded)
		assert.Equal(t, "QDNET", client.toolName)
		baseline := downloaded.BaselinePath()
		require.NotEmpty(t, baseline)
		inCacheDir, err := filepath.Rel(cacheDir, baseline)
		require.NoError(t, err)
		assert.NotContains(t, inCacheDir, "..", "the baseline should be stored under the cache dir")

		stored, err := ReadReport(baseline)
		require.NoError(t, err)
		assert.Equal(t, "QDNET", stored.Runs[0].Tool.Driver.Name)

		downloaded.Cleanup()
		assert.NoDirExists(t, filepath.Dir(baseline))
	})

	for _, tc := range []struct {
		name   string
		status cloud.BaselineStatus
	}{
		{"empty baseline in the cloud", cloud.BaselineEmpty},
		{"cloud baseline turned off", cloud.BaselineDisabled},
		{"cloud without baselines", cloud.BaselineUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cacheDir := t.TempDir()
			downloaded, err := downloadCloudBaseline(&baselineDownloaderStub{status: tc.status}, "QDNET", cacheDir)
			require.NoError(t, err)
			require.IsType(t, NoBaseline{}, downloaded)
			assert.Empty(t, downloaded.BaselinePath())
			left, err := os.ReadDir(cacheDir)
			require.NoError(t, err)
			assert.Empty(t, left, "no baseline dir should be left behind")
		})
	}

	t.Run("failed request", func(t *testing.T) {
		client := &baselineDownloaderStub{err: errors.New("the cloud is unreachable")}
		cacheDir := t.TempDir()

		downloaded, err := downloadCloudBaseline(client, "QDNET", cacheDir)

		assert.ErrorContains(t, err, "the cloud is unreachable")
		assert.Nil(t, downloaded)
		left, readErr := os.ReadDir(cacheDir)
		require.NoError(t, readErr)
		assert.Empty(t, left, "no baseline dir should be left behind")
	})

	// the cache dir of a container run is created by the linter, not before it
	t.Run("missing cache dir is created", func(t *testing.T) {
		client := &baselineDownloaderStub{baseline: reportWithToolName("QDNET")}
		cacheDir := filepath.Join(t.TempDir(), "absent")

		downloaded, err := downloadCloudBaseline(client, "QDNET", cacheDir)
		require.NoError(t, err)
		defer downloaded.Cleanup()

		assert.FileExists(t, downloaded.BaselinePath())
		assert.True(t, strings.HasPrefix(downloaded.BaselinePath(), cacheDir))
	})
}

func TestResolveBaseline(t *testing.T) {
	t.Run("baseline file wins over the cloud baseline", func(t *testing.T) {
		client := &baselineDownloaderStub{baseline: reportWithToolName("QDJVM")}
		baselineFile := filepath.Join(t.TempDir(), "baseline.sarif.json")

		baseline := resolveBaseline(baselineFile, true, "QDJVM", t.TempDir(), client.newClient)
		defer baseline.Cleanup()

		assert.False(t, client.asked)
		assert.Equal(t, LocalBaseline{path: baselineFile}, baseline)
		assert.Equal(t, "The analysis used the baseline file "+baselineFile, baseline.UsedMessage())
	})

	t.Run("no baseline without a cloud token", func(t *testing.T) {
		baseline := ResolveBaseline("", "", "QDJVM", t.TempDir())
		defer baseline.Cleanup()

		assert.Equal(t, NoBaseline{}, baseline)
		assert.Contains(t, baseline.UsedMessage(), "The analysis used no baseline.")
	})

	t.Run("baseline from the cloud", func(t *testing.T) {
		client := &baselineDownloaderStub{baseline: reportWithToolName("QDJVM")}

		baseline := resolveBaseline("", true, "QDJVMC", t.TempDir(), client.newClient)
		defer baseline.Cleanup()

		assert.Equal(t, "QDJVM", client.toolName)
		require.IsType(t, CloudBaseline{}, baseline)
		assert.FileExists(t, baseline.BaselinePath())
	})

	t.Run("a failed download is no baseline which tells so", func(t *testing.T) {
		client := &baselineDownloaderStub{err: errors.New("the cloud is unreachable")}

		baseline := resolveBaseline("", true, "QDJVM", t.TempDir(), client.newClient)

		assert.Equal(
			t,
			"The analysis used no baseline: the baseline of Qodana Cloud could not be downloaded.",
			baseline.UsedMessage(),
		)
	})

	for _, tc := range []struct {
		name   string
		status cloud.BaselineStatus
		used   string
	}{
		{
			"empty cloud baseline",
			cloud.BaselineEmpty,
			"The analysis used no baseline: the cloud baseline of the project in Qodana Cloud is empty.",
		},
		{
			"cloud baseline turned off",
			cloud.BaselineDisabled,
			"The analysis used no baseline. Give a baseline file with --baseline, " +
				"or turn on the cloud baseline of the project in Qodana Cloud.",
		},
		{
			"cloud without baselines",
			cloud.BaselineUnsupported,
			"The analysis used no baseline: this Qodana Cloud doesn't serve cloud baselines. " +
				"Give a baseline file with --baseline.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &baselineDownloaderStub{status: tc.status}

			baseline := resolveBaseline("", true, "QDJVM", t.TempDir(), client.newClient)
			defer baseline.Cleanup()

			require.IsType(t, NoBaseline{}, baseline)
			assert.Equal(t, tc.used, baseline.UsedMessage())
		})
	}
}

func TestUsedMessage(t *testing.T) {
	assert.Equal(
		t,
		"The analysis used the baseline from Qodana Cloud",
		CloudBaseline{path: "/cache/cloud-baseline-1/qodana.sarif.json"}.UsedMessage(),
	)
	assert.Equal(
		t,
		"The analysis used the baseline file /project/baseline.sarif.json",
		LocalBaseline{path: "/project/baseline.sarif.json"}.UsedMessage(),
	)
	assert.Equal(
		t,
		"The analysis used no baseline. Give a baseline file with --baseline, "+
			"or turn on the cloud baseline of the project in Qodana Cloud.",
		NoBaseline{}.UsedMessage(),
	)
}
