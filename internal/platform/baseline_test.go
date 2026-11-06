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
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
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

func (s *baselineDownloaderStub) WriteBaseline(toolName string, file *os.File) (cloud.BaselineStatus, error) {
	s.asked = true
	s.toolName = toolName
	if s.err != nil {
		return 0, s.err
	}
	if s.baseline == nil {
		return s.status, nil
	}
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

		baseline, status, cleanup, err := downloadCloudBaseline(client, "QDNET", cacheDir)
		require.NoError(t, err)
		defer cleanup()

		assert.Equal(t, cloud.BaselineWritten, status)
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
		dir, err := os.Stat(filepath.Dir(baseline))
		require.NoError(t, err)
		assert.Equal(
			t,
			openToOthers,
			dir.Mode().Perm()&openToOthers,
			"the baseline dir should be readable by the linter",
		)
		// os.Create obeys the umask, which a container user reading the file doesn't share
		const readableByOthers = os.FileMode(0o044)
		report, err := os.Stat(baseline)
		require.NoError(t, err)
		assert.Equal(
			t,
			readableByOthers,
			report.Mode().Perm()&readableByOthers,
			"the baseline file should be readable by the linter",
		)

		cleanup()
		assert.NoFileExists(t, baseline)
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
			baseline, status, cleanup, err := downloadCloudBaseline(
				&baselineDownloaderStub{status: tc.status},
				"QDNET",
				cacheDir,
			)
			defer cleanup()
			require.NoError(t, err)
			assert.Equal(t, tc.status, status)
			assert.Empty(t, baseline)
			left, err := os.ReadDir(cacheDir)
			require.NoError(t, err)
			assert.Empty(t, left, "no baseline dir should be left behind")
		})
	}

	t.Run("failed request", func(t *testing.T) {
		client := &baselineDownloaderStub{err: errors.New("the cloud is unreachable")}
		baseline, status, cleanup, err := downloadCloudBaseline(client, "QDNET", t.TempDir())
		defer cleanup()
		assert.ErrorContains(t, err, "the cloud is unreachable")
		assert.Zero(t, status)
		assert.Empty(t, baseline)
	})

	// the cache dir of a container run is created by the linter, not before it
	t.Run("missing cache dir is created", func(t *testing.T) {
		client := &baselineDownloaderStub{baseline: reportWithToolName("QDNET")}
		cacheDir := filepath.Join(t.TempDir(), "absent")

		baseline, status, cleanup, err := downloadCloudBaseline(client, "QDNET", cacheDir)
		defer cleanup()

		require.NoError(t, err)
		assert.Equal(t, cloud.BaselineWritten, status)
		assert.FileExists(t, baseline)
		assert.True(t, strings.HasPrefix(baseline, cacheDir))
	})
}

func TestResolveBaseline(t *testing.T) {
	t.Run("baseline file wins over the cloud baseline", func(t *testing.T) {
		baselineFile := filepath.Join(t.TempDir(), "baseline.sarif.json")
		require.NoError(t, os.WriteFile(baselineFile, []byte("{}"), 0644))

		baseline := ResolveBaseline(baselineFile, "", "token", "QDJVM", t.TempDir())
		defer baseline.Cleanup()

		assert.Equal(t, baselineFile, baseline.BaselinePath())
		assert.False(t, baseline.IsFromCloud())
		assert.Equal(t, "The analysis used the baseline file "+baselineFile, baseline.UsedMessage())
	})

	// the CLI downloads the baseline and passes it as a file, saying where it comes from
	t.Run("baseline file of the launching CLI is the cloud baseline", func(t *testing.T) {
		baselineFile := filepath.Join(t.TempDir(), "baseline.sarif.json")
		require.NoError(t, os.WriteFile(baselineFile, []byte("{}"), 0644))

		baseline := ResolveBaseline(baselineFile, qdenv.BaselineSourceCloud, "token", "QDJVM", t.TempDir())
		defer baseline.Cleanup()

		assert.Equal(t, baselineFile, baseline.BaselinePath())
		assert.True(t, baseline.IsFromCloud())
		assert.Equal(t, qdenv.BaselineSourceCloud, baseline.Source())
		assert.Equal(t, "The analysis used the baseline from Qodana Cloud", baseline.UsedMessage())
	})

	// the source of a linter which the CLI told nothing is its own command line
	t.Run("baseline file of a run without a source is a local one", func(t *testing.T) {
		baselineFile := filepath.Join(t.TempDir(), "baseline.sarif.json")
		require.NoError(t, os.WriteFile(baselineFile, []byte("{}"), 0644))

		baseline := ResolveBaseline(baselineFile, "", "token", "QDJVM", t.TempDir())
		defer baseline.Cleanup()

		assert.False(t, baseline.IsFromCloud())
		assert.Equal(t, qdenv.BaselineSourceLocal, baseline.Source())
	})

	// the outer CLI of a linter in an image has asked Qodana Cloud already
	t.Run("no second ask when a CLI has resolved the baseline", func(t *testing.T) {
		client := &baselineDownloaderStub{baseline: reportWithToolName("QDJVM")}

		baseline := resolveBaseline("", qdenv.BaselineSourceNone, true, "QDJVM", t.TempDir(), client.newClient)
		defer baseline.Cleanup()

		assert.False(t, client.asked)
		assert.Empty(t, baseline.BaselinePath())
		assert.Equal(t, qdenv.BaselineSourceNone, baseline.Source())
	})

	// a CLI which has resolved a baseline passes its file, so a source without one is not that
	t.Run("source of a baseline without its file is ignored", func(t *testing.T) {
		for _, source := range []string{qdenv.BaselineSourceLocal, qdenv.BaselineSourceCloud, "UNKNOWN"} {
			t.Run(source, func(t *testing.T) {
				client := &baselineDownloaderStub{baseline: reportWithToolName("QDJVM")}

				baseline := resolveBaseline("", source, true, "QDJVM", t.TempDir(), client.newClient)
				defer baseline.Cleanup()

				assert.True(t, client.asked, "Qodana Cloud should be asked for the baseline")
				assert.True(t, baseline.IsFromCloud())
				assert.FileExists(t, baseline.BaselinePath())
			})
		}
	})

	t.Run("no baseline without a cloud token", func(t *testing.T) {
		baseline := ResolveBaseline("", "", "", "QDJVM", t.TempDir())
		defer baseline.Cleanup()

		assert.Empty(t, baseline.BaselinePath())
		assert.Equal(t, qdenv.BaselineSourceNone, baseline.Source())
		assert.Contains(t, baseline.UsedMessage(), "The analysis used no baseline.")
	})

	t.Run("baseline from the cloud", func(t *testing.T) {
		client := &baselineDownloaderStub{baseline: reportWithToolName("QDJVM")}

		baseline := resolveBaseline("", "", true, "QDJVMC", t.TempDir(), client.newClient)
		defer baseline.Cleanup()

		assert.Equal(t, "QDJVM", client.toolName)
		assert.True(t, baseline.IsFromCloud())
		assert.FileExists(t, baseline.BaselinePath())
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
			"The analysis used no baseline. Give a baseline file with --baseline: " +
				"this Qodana Cloud doesn't serve cloud baselines.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &baselineDownloaderStub{status: tc.status}

			baseline := resolveBaseline("", "", true, "QDJVM", t.TempDir(), client.newClient)
			defer baseline.Cleanup()

			assert.Empty(t, baseline.BaselinePath())
			assert.Equal(t, qdenv.BaselineSourceNone, baseline.Source())
			assert.Equal(t, tc.used, baseline.UsedMessage())
		})
	}
}

func TestUsedMessage(t *testing.T) {
	cloudBaseline := Baseline{baselinePath: "/cache/cloud-baseline-1/qodana.sarif.json", source: qdenv.BaselineSourceCloud}
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
