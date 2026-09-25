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

	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/JetBrains/qodana-cli/internal/sarif"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// baselineDownloaderStub writes the baseline it is given, the way the cloud client does.
type baselineDownloaderStub struct {
	toolName string
	baseline *sarif.Report
	err      error
}

func (s *baselineDownloaderStub) WriteBaseline(toolName string, file *os.File) (bool, error) {
	s.toolName = toolName
	if s.err != nil || s.baseline == nil {
		return false, s.err
	}
	return true, json.NewEncoder(file).Encode(s.baseline)
}

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

		baseline, cleanup, err := downloadCloudBaseline(client, "QDNET", cacheDir)
		require.NoError(t, err)
		defer cleanup()

		assert.Equal(t, "QDNET", client.toolName)
		require.NotEmpty(t, baseline)
		inCacheDir, err := filepath.Rel(cacheDir, baseline)
		require.NoError(t, err)
		assert.NotContains(t, inCacheDir, "..", "the baseline should be stored under the cache dir")

		stored, err := ReadReport(baseline)
		require.NoError(t, err)
		assert.Equal(t, "QDNET", stored.Runs[0].Tool.Driver.Name)

		// the linter of a container run reads it as another user. Only the bits which let one in
		// are asserted: Windows reports 0777 for a directory whatever it was created with.
		const openToOthers = os.FileMode(0o055)
		info, err := os.Stat(filepath.Dir(baseline))
		require.NoError(t, err)
		assert.Equal(
			t,
			openToOthers,
			info.Mode().Perm()&openToOthers,
			"the baseline dir should be readable by the linter",
		)

		cleanup()
		assert.NoFileExists(t, baseline)
	})

	t.Run("no baseline in the cloud", func(t *testing.T) {
		baseline, cleanup, err := downloadCloudBaseline(&baselineDownloaderStub{}, "QDNET", t.TempDir())
		defer cleanup()
		require.NoError(t, err)
		assert.Empty(t, baseline)
	})

	t.Run("failed request", func(t *testing.T) {
		client := &baselineDownloaderStub{err: errors.New("the cloud is unreachable")}
		baseline, cleanup, err := downloadCloudBaseline(client, "QDNET", t.TempDir())
		defer cleanup()
		assert.ErrorContains(t, err, "the cloud is unreachable")
		assert.Empty(t, baseline)
	})

	// the cache dir of a container run is created by the linter, not before it
	t.Run("missing cache dir is created", func(t *testing.T) {
		client := &baselineDownloaderStub{baseline: reportWithToolName("QDNET")}
		cacheDir := filepath.Join(t.TempDir(), "absent")

		baseline, cleanup, err := downloadCloudBaseline(client, "QDNET", cacheDir)
		defer cleanup()

		require.NoError(t, err)
		assert.FileExists(t, baseline)
		assert.True(t, strings.HasPrefix(baseline, cacheDir))
	})
}

func TestResolveBaseline(t *testing.T) {
	t.Run("baseline file wins over the cloud baseline", func(t *testing.T) {
		baselineFile := filepath.Join(t.TempDir(), "baseline.sarif.json")
		require.NoError(t, os.WriteFile(baselineFile, []byte("{}"), 0644))

		baseline := ResolveBaseline(baselineFile, "token", "QDJVM", t.TempDir())
		defer baseline.Cleanup()

		assert.Equal(t, baselineFile, baseline.BaselinePath())
		assert.False(t, baseline.IsFromCloud())
		assert.Equal(t, "The analysis used the baseline file "+baselineFile, baseline.UsedMessage())
	})

	// the CLI downloads the baseline and passes it as a file, saying where it comes from
	t.Run("baseline file of the launching CLI is the cloud baseline", func(t *testing.T) {
		t.Setenv(qdenv.QodanaBaselineFromCloud, "true")
		baselineFile := filepath.Join(t.TempDir(), "baseline.sarif.json")
		require.NoError(t, os.WriteFile(baselineFile, []byte("{}"), 0644))

		baseline := ResolveBaseline(baselineFile, "token", "QDJVM", t.TempDir())
		defer baseline.Cleanup()

		assert.Equal(t, baselineFile, baseline.BaselinePath())
		assert.True(t, baseline.IsFromCloud())
		assert.Equal(t, "The analysis used the baseline from Qodana Cloud", baseline.UsedMessage())
	})

	t.Run("no baseline without a cloud token", func(t *testing.T) {
		baseline := ResolveBaseline("", "", "QDJVM", t.TempDir())
		defer baseline.Cleanup()

		assert.Empty(t, baseline.BaselinePath())
		assert.Contains(t, baseline.UsedMessage(), "The analysis used no baseline.")
	})
}

func TestUsedMessage(t *testing.T) {
	cloudBaseline := Baseline{baselinePath: "/cache/cloud-baseline-1/qodana.sarif.json", isCloudBaseline: true}
	assert.Equal(t, "The analysis used the baseline from Qodana Cloud", cloudBaseline.UsedMessage())
	assert.Equal(
		t,
		"The analysis used the baseline file /project/baseline.sarif.json",
		Baseline{baselinePath: "/project/baseline.sarif.json"}.UsedMessage(),
	)
	assert.Equal(
		t,
		"The analysis used no baseline. Give a baseline file with --baseline, "+
			"or turn on the cloud baseline of the project in Qodana Cloud.",
		Baseline{}.UsedMessage(),
	)
}
