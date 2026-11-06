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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/cloud"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cloudAnswering returns the baseline client of a Qodana Cloud which answers the request of a baseline
// with status and body. The tool name it is asked for is sent to toolNames.
func cloudAnswering(t *testing.T, status int, body string) (client *cloud.QdClient, toolNames <-chan string) {
	asked := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case asked <- r.URL.Query().Get("toolName"):
		default:
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return (&cloud.QdApiEndpoints{LintersApiUrl: server.URL}).NewBaselineClient("token"), asked
}

// unreachableCloud returns the baseline client of a Qodana Cloud which fails every request, tried once.
func unreachableCloud(t *testing.T) *cloud.QdClient {
	t.Setenv(qdenv.QodanaCloudRequestRetriesEnv, "1")
	client, _ := cloudAnswering(t, http.StatusInternalServerError, "down")
	return client
}

// cloudWithBaseline returns the baseline client of a Qodana Cloud which has a baseline of one problem.
func cloudWithBaseline(t *testing.T) (client *cloud.QdClient, toolNames <-chan string) {
	return cloudAnswering(t, http.StatusOK, `{"baseline":[{"ruleId":"UnusedSymbol","message":{"text":"unused"}}]}`)
}

// launchedBy makes this CLI one in a container, launched by a CLI which has resolved source for it. An
// empty source makes it a CLI on a host, which nothing has launched.
func launchedBy(t *testing.T, source string) {
	if source == "" {
		notLaunched(t)
		return
	}
	t.Setenv(qdenv.QodanaDockerEnv, "true")
	t.Setenv(qdenv.QodanaBaselineSource, source)
}

// notLaunched makes this CLI one on a host, which nothing has launched, whatever the environment of the
// test run is.
func notLaunched(t *testing.T) {
	t.Setenv(qdenv.QodanaDockerEnv, "")
	t.Setenv(qdenv.QodanaBaselineSource, "")
}

func TestDownloadCloudBaseline(t *testing.T) {
	// the system temp dir is small and sometimes backed by memory, the baseline can be huge
	t.Run("baseline is stored in the cache dir", func(t *testing.T) {
		ownerOnlyUmask(t)
		client, toolNames := cloudWithBaseline(t)
		cacheDir := t.TempDir()

		downloaded, err := downloadCloudBaseline(client, "QDNET", cacheDir)
		require.NoError(t, err)
		defer downloaded.Cleanup()

		assert.IsType(t, CloudBaseline{}, downloaded)
		assert.Equal(t, "QDNET", <-toolNames)
		baseline := downloaded.BaselinePath()
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

		downloaded.Cleanup()
		assert.NoDirExists(t, filepath.Dir(baseline), "the directory of the download should be removed")
	})

	for _, tc := range []struct {
		name   string
		status int
	}{
		{"empty baseline in the cloud", http.StatusOK},
		{"cloud baseline turned off", http.StatusNoContent},
		{"cloud without baselines", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cacheDir := t.TempDir()
			client, _ := cloudAnswering(t, tc.status, "")
			downloaded, err := downloadCloudBaseline(client, "QDNET", cacheDir)
			require.NoError(t, err)
			// the reason it tells is checked by TestResolveBaseline
			assert.IsType(t, NoBaseline{}, downloaded)
			left, err := os.ReadDir(cacheDir)
			require.NoError(t, err)
			assert.Empty(t, left, "no baseline dir should be left behind")
		})
	}

	t.Run("failed request", func(t *testing.T) {
		downloaded, err := downloadCloudBaseline(unreachableCloud(t), "QDNET", t.TempDir())
		assert.ErrorContains(t, err, "baseline request failed")
		assert.Nil(t, downloaded)
	})

	// the cache dir of a container run is created by the linter, not before it
	t.Run("missing cache dir is created", func(t *testing.T) {
		client, _ := cloudWithBaseline(t)
		cacheDir := filepath.Join(t.TempDir(), "absent")

		downloaded, err := downloadCloudBaseline(client, "QDNET", cacheDir)
		require.NoError(t, err)
		defer downloaded.Cleanup()

		assert.FileExists(t, downloaded.BaselinePath())
		assert.True(t, strings.HasPrefix(downloaded.BaselinePath(), cacheDir))
	})
}

func TestResolveBaseline(t *testing.T) {
	notLaunched(t)

	t.Run("baseline file wins over the cloud baseline", func(t *testing.T) {
		baselineFile := filepath.Join(t.TempDir(), "baseline.sarif.json")
		require.NoError(t, os.WriteFile(baselineFile, []byte("{}"), 0644))

		baseline := ResolveBaseline(baselineFile, "token", "QDJVM", t.TempDir())
		defer baseline.Cleanup()

		assert.Equal(t, baselineFile, baseline.BaselinePath())
		assert.IsType(t, LocalBaseline{}, baseline)
		assert.Equal(t, "The analysis used the baseline file "+baselineFile, baseline.usedMessage())
	})

	// the CLI downloads the baseline and passes it as a file, saying where it comes from
	t.Run("baseline file of the launching CLI is the cloud baseline", func(t *testing.T) {
		baselineFile := filepath.Join(t.TempDir(), "baseline.sarif.json")
		require.NoError(t, os.WriteFile(baselineFile, []byte("{}"), 0644))

		launchedBy(t, qdenv.BaselineSourceCloud)
		baseline := ResolveBaseline(baselineFile, "token", "QDJVM", t.TempDir())

		assert.Equal(t, baselineFile, baseline.BaselinePath())
		assert.IsType(t, CloudBaseline{}, baseline)
		assert.Equal(t, qdenv.BaselineSourceCloud, baseline.Source())
		assert.Equal(t, "The analysis used the baseline from Qodana Cloud", baseline.usedMessage())
		baseline.Cleanup()
		assert.FileExists(t, baselineFile, "the file of the launching CLI is removed by that CLI")
	})

	// the source of a linter which the CLI told nothing is its own command line
	t.Run("baseline file of a run without a source is a local one", func(t *testing.T) {
		baselineFile := filepath.Join(t.TempDir(), "baseline.sarif.json")
		require.NoError(t, os.WriteFile(baselineFile, []byte("{}"), 0644))

		baseline := ResolveBaseline(baselineFile, "token", "QDJVM", t.TempDir())
		defer baseline.Cleanup()

		assert.IsType(t, LocalBaseline{}, baseline)
		assert.Equal(t, qdenv.BaselineSourceLocal, baseline.Source())
	})

	// the outer CLI of a linter in an image has asked Qodana Cloud already
	t.Run("no second ask when a CLI has resolved the baseline", func(t *testing.T) {
		launchedBy(t, qdenv.BaselineSourceNone)
		baseline, given := givenBaseline("")

		assert.True(t, given, "Qodana Cloud should not be asked again")
		assert.Empty(t, baseline.BaselinePath())
		assert.Equal(t, qdenv.BaselineSourceNone, baseline.Source())
	})

	// a CLI which has resolved a baseline passes its file, so a source without one is not that
	t.Run("source of a baseline without its file is ignored", func(t *testing.T) {
		for _, source := range []string{qdenv.BaselineSourceLocal, qdenv.BaselineSourceCloud, "UNKNOWN"} {
			t.Run(source, func(t *testing.T) {
				launchedBy(t, source)
				_, given := givenBaseline("")

				assert.False(t, given, "Qodana Cloud should be asked for the baseline")
			})
		}
	})

	t.Run("no baseline without a cloud token", func(t *testing.T) {
		baseline := ResolveBaseline("", "", "QDJVM", t.TempDir())
		defer baseline.Cleanup()

		assert.Empty(t, baseline.BaselinePath())
		assert.Equal(t, qdenv.BaselineSourceNone, baseline.Source())
		assert.Equal(
			t,
			"The analysis used no baseline. Give a baseline file with --baseline, "+
				"or run with the token of a Qodana Cloud project which has the cloud baseline turned on.",
			baseline.usedMessage(),
		)
	})

	t.Run("baseline from the cloud", func(t *testing.T) {
		client, toolNames := cloudWithBaseline(t)

		baseline := askCloudBaseline(client, "QDJVMC", t.TempDir())
		defer baseline.Cleanup()

		assert.Equal(t, "QDJVM", <-toolNames)
		assert.IsType(t, CloudBaseline{}, baseline)
		assert.FileExists(t, baseline.BaselinePath())
		assert.True(t, baseline.reportsUsed(true), "the host reports the baseline it has downloaded")
	})

	t.Run("cloud baseline which cannot be downloaded", func(t *testing.T) {
		baseline := askCloudBaseline(unreachableCloud(t), "QDJVM", t.TempDir())

		assert.Equal(
			t,
			"The analysis used no baseline. The baseline of Qodana Cloud could not be downloaded.",
			baseline.usedMessage(),
		)
	})

	for _, tc := range []struct {
		name   string
		status int
		used   string
	}{
		{
			"empty cloud baseline",
			http.StatusOK,
			"The analysis used no baseline. The cloud baseline of the project in Qodana Cloud is empty.",
		},
		{
			"cloud baseline turned off",
			http.StatusNoContent,
			"The analysis used no baseline. The cloud baseline is turned off for this project in Qodana Cloud. " +
				"Give a baseline file with --baseline, or turn on the cloud baseline.",
		},
		{
			"cloud without baselines",
			http.StatusNotFound,
			"The analysis used no baseline. This Qodana Cloud doesn't serve cloud baselines. " +
				"Give a baseline file with --baseline.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := cloudAnswering(t, tc.status, "")

			baseline := askCloudBaseline(client, "QDJVM", t.TempDir())
			defer baseline.Cleanup()

			assert.Empty(t, baseline.BaselinePath())
			assert.Equal(t, qdenv.BaselineSourceNone, baseline.Source())
			assert.Equal(t, tc.used, baseline.usedMessage())
		})
	}
}

// TestLauncherBaselineSource verifies that only a CLI in a Qodana image takes the baseline source
// of the CLI which launched it: on a host the variable is a leftover of another run.
func TestLauncherBaselineSource(t *testing.T) {
	for _, tc := range []struct {
		name      string
		container bool
		source    string
		expected  string
	}{
		{"container, cloud baseline", true, qdenv.BaselineSourceCloud, qdenv.BaselineSourceCloud},
		{"container, no baseline", true, qdenv.BaselineSourceNone, qdenv.BaselineSourceNone},
		{"container launched without a CLI", true, "", ""},
		{"host, leftover cloud baseline", false, qdenv.BaselineSourceCloud, ""},
		{"host, leftover no baseline", false, qdenv.BaselineSourceNone, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			docker := ""
			if tc.container {
				docker = "true"
			}
			t.Setenv(qdenv.QodanaDockerEnv, docker)
			t.Setenv(qdenv.QodanaBaselineSource, tc.source)

			assert.Equal(t, tc.expected, launcherBaselineSource())
		})
	}
}

// TestReportsUsed verifies that a run reports the baseline it has used once: the host reports it,
// except for a local baseline, which the CLI in the container reports with its path.
func TestReportsUsed(t *testing.T) {
	baselineFile := filepath.Join(t.TempDir(), "qodana.sarif.json")
	for _, tc := range []struct {
		name              string
		baselineFile      string
		launcherSource    string
		linterInContainer bool
		expected          bool
	}{
		{"host, native linter, no baseline", "", "", false, true},
		{"host, native linter, local baseline", baselineFile, "", false, true},
		{"host, no baseline for a container", "", "", true, true},
		{"host, local baseline for a container", baselineFile, "", true, false},
		{"container, cloud baseline from the host", baselineFile, qdenv.BaselineSourceCloud, false, false},
		{"container, no baseline from the host", "", qdenv.BaselineSourceNone, false, false},
		{"container, local baseline from the host", baselineFile, qdenv.BaselineSourceLocal, false, true},
		// the source is ignored, so the CLI resolves the baseline itself, like one which has no launcher
		{"container, source without a file", "", qdenv.BaselineSourceCloud, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			launchedBy(t, tc.launcherSource)
			baseline := ResolveBaseline(tc.baselineFile, "", "QDJVM", t.TempDir())
			defer baseline.Cleanup()

			assert.Equal(t, tc.expected, baseline.reportsUsed(tc.linterInContainer))
		})
	}
}

func TestUsedMessage(t *testing.T) {
	cloudBaseline := CloudBaseline{path: "/cache/cloud-baseline-1/qodana.sarif.json"}
	assert.Equal(t, "The analysis used the baseline from Qodana Cloud", cloudBaseline.usedMessage())
	assert.Equal(
		t,
		"The analysis used the baseline file /project/baseline.sarif.json",
		LocalBaseline{path: "/project/baseline.sarif.json"}.usedMessage(),
	)
}
