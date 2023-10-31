/*
 * Copyright 2021-2023 JetBrains s.r.o.
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

package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2023/cloud"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/owenrumney/go-sarif/v2/sarif"
	"github.com/stretchr/testify/assert"
)

func TestCliArgs(t *testing.T) {
	dir, _ := os.Getwd()
	projectDir := filepath.Join(dir, "project")
	cacheDir := filepath.Join(dir, "cache")
	resultsDir := filepath.Join(dir, "results")
	prod.Home = string(os.PathSeparator) + "opt" + string(os.PathSeparator) + "idea"
	prod.IdeScript = filepath.Join(prod.Home, "bin", "idea.sh")
	err := os.Unsetenv(qodanaDockerEnv)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		opts *QodanaOptions
		res  []string
	}{
		{
			name: "typical set up",
			opts: &QodanaOptions{ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, Linter: "jetbrains/qodana-jvm-community:latest", SourceDirectory: "./src", DisableSanity: true, RunPromo: "true", Baseline: "qodana.sarif.json", BaselineIncludeAbsent: true, SaveReport: true, ShowReport: true, Port: 8888, Property: []string{"foo.baz=bar", "foo.bar=baz"}, Script: "default", FailThreshold: "0", AnalysisId: "id", Env: []string{"A=B"}, Volumes: []string{"/tmp/foo:/tmp/foo"}, User: "1001:1001", PrintProblems: true, ProfileName: "Default", ApplyFixes: true},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--save-report", "--source-directory", "./src", "--disable-sanity", "--profile-name", "Default", "--run-promo", "true", "--baseline", "qodana.sarif.json", "--baseline-include-absent", "--fail-threshold", "0", "--fixes-strategy", "apply", "--analysis-id", "id", "--property=foo.baz=bar", "--property=foo.bar=baz", projectDir, resultsDir},
		},
		{
			name: "arguments with spaces, no properties for local runs",
			opts: &QodanaOptions{ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, ProfileName: "separated words", Property: []string{"qodana.format=SARIF_AND_PROJECT_STRUCTURE", "qodana.variable.format=JSON"}, Ide: prod.Home},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--profile-name", "\"separated words\"", projectDir, resultsDir},
		},
		{
			name: "deprecated --fixes-strategy=apply",
			opts: &QodanaOptions{ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, FixesStrategy: "apply"},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--fixes-strategy", "apply", projectDir, resultsDir},
		},
		{
			name: "deprecated --fixes-strategy=cleanup",
			opts: &QodanaOptions{ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, FixesStrategy: "cleanup"},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--fixes-strategy", "cleanup", projectDir, resultsDir},
		},
		{
			name: "--fixes-strategy=apply for new versions",
			opts: &QodanaOptions{ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, FixesStrategy: "apply", Ide: "/opt/idea/233"},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--apply-fixes", projectDir, resultsDir},
		},
		{
			name: "--fixes-strategy=cleanup for new versions",
			opts: &QodanaOptions{ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, FixesStrategy: "cleanup", Ide: "/opt/idea/233"},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--cleanup", projectDir, resultsDir},
		},
		{
			name: "--stub-profile ignored",
			opts: &QodanaOptions{StubProfile: "ignored", ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, FixesStrategy: "cleanup", Ide: "/opt/idea/233"},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--cleanup", projectDir, resultsDir},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.opts.Ide == "/opt/idea/233" {
				prod.Version = "2023.3"
			} else {
				prod.Version = "2023.2"
			}

			args := getIdeRunCommand(tc.opts)
			assert.Equal(t, tc.res, args)
		})
	}
}

func TestCloudUrl(t *testing.T) {
	for _, tc := range []struct {
		name        string
		writtenUrl  string
		expectedUrl string
	}{
		{
			name:        "valid url",
			writtenUrl:  "https://youtu.be/dQw4w9WgXcQ",
			expectedUrl: "https://youtu.be/dQw4w9WgXcQ",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resultsPath := filepath.Join(os.TempDir(), "cloud_url_valid")
			err := os.MkdirAll(resultsPath, 0o755)
			if err != nil {
				return
			}

			filePath := resultsPath + "/" + qodanaReportUrlFile
			err = os.WriteFile(
				filePath,
				[]byte(tc.writtenUrl),
				0o644,
			)
			if err != nil {
				t.Fatal(err)
			}
			actual := getReportUrl(resultsPath)
			if actual != tc.expectedUrl {
				t.Fatalf("expected \"%s\" got \"%s\"", tc.expectedUrl, actual)
			}
		})
	}
}

func unsetGitHubVariables() {
	variables := []string{
		"GITHUB_SERVER_URL",
		"GITHUB_REPOSITORY",
		"GITHUB_RUN_ID",
		"GITHUB_HEAD_REF",
		"GITHUB_REF",
	}
	for _, v := range variables {
		_ = os.Unsetenv(v)
	}
}

func Test_ExtractEnvironmentVariables(t *testing.T) {
	revisionExpected := "1234567890abcdef1234567890abcdef12345678"
	branchExpected := "refs/heads/main"

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		unsetGitHubVariables()
	}

	for _, tc := range []struct {
		ci                string
		variables         map[string]string
		jobUrlExpected    string
		envExpected       string
		remoteUrlExpected string
		revisionExpected  string
		branchExpected    string
	}{
		{
			ci:          "no CI detected",
			variables:   map[string]string{},
			envExpected: "cli:dev",
		},
		{
			ci: "User defined",
			variables: map[string]string{
				qodanaEnv:       "user-defined",
				qodanaJobUrl:    "https://qodana.jetbrains.com/never-gonna-give-you-up",
				qodanaRemoteUrl: "https://qodana.jetbrains.com/never-gonna-give-you-up",
				qodanaBranch:    branchExpected,
				qodanaRevision:  revisionExpected,
			},
			envExpected:       "user-defined",
			remoteUrlExpected: "https://qodana.jetbrains.com/never-gonna-give-you-up",
			jobUrlExpected:    "https://qodana.jetbrains.com/never-gonna-give-you-up",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "Space",
			variables: map[string]string{
				"JB_SPACE_EXECUTION_URL":       "https://space.jetbrains.com/never-gonna-give-you-up",
				"JB_SPACE_GIT_BRANCH":          branchExpected,
				"JB_SPACE_GIT_REVISION":        revisionExpected,
				"JB_SPACE_API_URL":             "jetbrains.team",
				"JB_SPACE_PROJECT_KEY":         "sa",
				"JB_SPACE_GIT_REPOSITORY_NAME": "entrypoint",
			},
			envExpected:       fmt.Sprintf("space:%s", Version),
			remoteUrlExpected: "ssh://git@git.jetbrains.team/sa/entrypoint.git",
			jobUrlExpected:    "https://space.jetbrains.com/never-gonna-give-you-up",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "GitLab",
			variables: map[string]string{
				"CI_JOB_URL":        "https://gitlab.jetbrains.com/never-gonna-give-you-up",
				"CI_COMMIT_BRANCH":  branchExpected,
				"CI_COMMIT_SHA":     revisionExpected,
				"CI_REPOSITORY_URL": "https://gitlab.jetbrains.com/sa/entrypoint.git",
			},
			envExpected:       fmt.Sprintf("gitlab:%s", Version),
			remoteUrlExpected: "https://gitlab.jetbrains.com/sa/entrypoint.git",
			jobUrlExpected:    "https://gitlab.jetbrains.com/never-gonna-give-you-up",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "Jenkins",
			variables: map[string]string{
				"BUILD_URL":        "https://jenkins.jetbrains.com/never-gonna-give-you-up",
				"GIT_LOCAL_BRANCH": branchExpected,
				"GIT_COMMIT":       revisionExpected,
				"GIT_URL":          "https://git.jetbrains.com/sa/entrypoint.git",
			},
			envExpected:       fmt.Sprintf("jenkins:%s", Version),
			jobUrlExpected:    "https://jenkins.jetbrains.com/never-gonna-give-you-up",
			remoteUrlExpected: "https://git.jetbrains.com/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "GitHub",
			variables: map[string]string{
				"GITHUB_SERVER_URL": "https://github.jetbrains.com",
				"GITHUB_REPOSITORY": "sa/entrypoint",
				"GITHUB_RUN_ID":     "123456789",
				"GITHUB_SHA":        revisionExpected,
				"GITHUB_HEAD_REF":   branchExpected,
			},
			envExpected:       fmt.Sprintf("github-actions:%s", Version),
			jobUrlExpected:    "https://github.jetbrains.com/sa/entrypoint/actions/runs/123456789",
			remoteUrlExpected: "https://github.jetbrains.com/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "GitHub push",
			variables: map[string]string{
				"GITHUB_SERVER_URL": "https://github.jetbrains.com",
				"GITHUB_REPOSITORY": "sa/entrypoint",
				"GITHUB_RUN_ID":     "123456789",
				"GITHUB_SHA":        revisionExpected,
				"GITHUB_REF":        branchExpected,
			},
			envExpected:       fmt.Sprintf("github-actions:%s", Version),
			jobUrlExpected:    "https://github.jetbrains.com/sa/entrypoint/actions/runs/123456789",
			remoteUrlExpected: "https://github.jetbrains.com/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "CircleCI",
			variables: map[string]string{
				"CIRCLE_BUILD_URL":      "https://circleci.jetbrains.com/never-gonna-give-you-up",
				"CIRCLE_SHA1":           revisionExpected,
				"CIRCLE_BRANCH":         branchExpected,
				"CIRCLE_REPOSITORY_URL": "https://circleci.jetbrains.com/sa/entrypoint.git",
			},
			envExpected:       fmt.Sprintf("circleci:%s", Version),
			jobUrlExpected:    "https://circleci.jetbrains.com/never-gonna-give-you-up",
			remoteUrlExpected: "https://circleci.jetbrains.com/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "Azure Pipelines",
			variables: map[string]string{
				"SYSTEM_TEAMFOUNDATIONCOLLECTIONURI": "https://dev.azure.com/jetbrains",
				"BUILD_BUILDURI":                     "https://dev.azure.com/jetbrains/never-gonna-give-you-up",
				"SYSTEM_TEAMPROJECT":                 "/sa",
				"BUILD_BUILDID":                      "123456789",
				"BUILD_SOURCEVERSION":                revisionExpected,
				"BUILD_SOURCEBRANCH":                 "refs/heads/" + branchExpected,
				"BUILD_REPOSITORY_URI":               "https://dev.azure.com/jetbrains/sa/entrypoint.git",
			},
			envExpected:       fmt.Sprintf("azure-pipelines:%s", Version),
			jobUrlExpected:    "https://dev.azure.com/jetbrains/sa/_build/results?buildId=123456789",
			remoteUrlExpected: "https://dev.azure.com/jetbrains/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
	} {
		t.Run(tc.ci, func(t *testing.T) {
			opts := &QodanaOptions{}
			for k, v := range tc.variables {
				err := os.Setenv(k, v)
				if err != nil {
					t.Fatal(err)
				}
				opts.setenv(k, v)
			}

			for _, environment := range []struct {
				name  string
				set   func(string, string)
				unset func(string)
				get   func(string) string
			}{
				{
					name: "Container",
					set:  opts.setenv,
					get:  opts.getenv,
				},
				{
					name: "Local",
					set:  setEnv,
					get:  os.Getenv,
				},
			} {
				t.Run(environment.name, func(t *testing.T) {
					ExtractQodanaEnvironment(environment.set)
					currentQodanaEnv := environment.get(qodanaEnv)
					if currentQodanaEnv != tc.envExpected {
						t.Errorf("%s: Expected %s, got %s", environment.name, tc.envExpected, currentQodanaEnv)
					}
					if environment.get(qodanaJobUrl) != tc.jobUrlExpected {
						t.Errorf("%s: Expected %s, got %s", environment.name, tc.jobUrlExpected, environment.get(qodanaJobUrl))
					}
					if environment.get(qodanaRemoteUrl) != tc.remoteUrlExpected {
						t.Errorf("%s: Expected %s, got %s", environment.name, tc.remoteUrlExpected, environment.get(qodanaRemoteUrl))
					}
					if environment.get(qodanaRevision) != tc.revisionExpected {
						t.Errorf("%s: Expected %s, got %s", environment.name, revisionExpected, environment.get(qodanaRevision))
					}
					if environment.get(qodanaBranch) != tc.branchExpected {
						t.Errorf("%s: Expected %s, got %s", environment.name, branchExpected, environment.get(qodanaBranch))
					}
				})
			}

			for _, k := range append(maps.Keys(tc.variables), []string{qodanaJobUrl, qodanaEnv, qodanaRemoteUrl, qodanaRevision, qodanaBranch}...) {
				err := os.Unsetenv(k)
				if err != nil {
					t.Fatal(err)
				}
				opts.unsetenv(k)
			}
		})
	}
}

func TestDirLanguagesExcluded(t *testing.T) {
	expected := []string{"Go", "Shell"}
	actual, err := recognizeDirLanguages("../")
	if err != nil {
		return
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}

func TestScanFlags_Script(t *testing.T) {
	testOptions := &QodanaOptions{
		Script: "custom-script:parameters",
	}
	expected := []string{
		"--script",
		"custom-script:parameters",
	}
	actual := getIdeArgs(testOptions)
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}

func TestLegacyFixStrategies(t *testing.T) {
	cases := []struct {
		name     string
		options  *QodanaOptions
		expected []string
	}{
		{
			name: "apply fixes for a container",
			options: &QodanaOptions{
				ApplyFixes: true,
				Ide:        "",
			},
			expected: []string{
				"--fixes-strategy",
				"apply",
			},
		},
		{
			name: "cleanup for a container",
			options: &QodanaOptions{
				Cleanup: true,
				Ide:     "",
			},
			expected: []string{
				"--fixes-strategy",
				"cleanup",
			},
		},
		{
			name: "apply fixes for new IDE",
			options: &QodanaOptions{
				ApplyFixes: true,
				Ide:        "QDPHP",
			},
			expected: []string{
				"--apply-fixes",
			},
		},
		{
			name: "fixes for unavailable IDE",
			options: &QodanaOptions{
				Cleanup: true,
				Ide:     "QDNET",
			},
			expected: []string{},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.options.Ide == "QDPHP" {
				prod.Version = "2023.3"
			} else {
				prod.Version = "2023.2"
			}

			actual := getIdeArgs(tt.options)
			if !reflect.DeepEqual(tt.expected, actual) {
				t.Fatalf("expected \"%s\" got \"%s\"", tt.expected, actual)
			}
		})
	}
}

func TestReadIdeaDir(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := os.TempDir()
	tempDir = filepath.Join(tempDir, "readIdeaDir")
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(tempDir)

	// Case 1: .idea directory with iml files for Java and Kotlin
	ideaDir := filepath.Join(tempDir, ".idea")
	err := os.MkdirAll(ideaDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	imlFile := filepath.Join(ideaDir, "test.iml")
	err = os.WriteFile(imlFile, []byte("<module type=\"JAVA_MODULE\"/>"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	kotlinImlFile := filepath.Join(ideaDir, "test.kt.iml")
	err = os.WriteFile(kotlinImlFile, []byte("<module type=\"JAVA_MODULE\" languageLevel=\"JDK_1_8\"/>"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	languages := readIdeaDir(tempDir)
	expected := []string{"Java"}
	if !reflect.DeepEqual(languages, expected) {
		t.Errorf("Case 1: Expected %v, but got %v", expected, languages)
	}

	// Case 2: .idea directory with no iml files
	err = os.Remove(imlFile)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Remove(kotlinImlFile)
	if err != nil {
		t.Fatal(err)
	}
	languages = readIdeaDir(tempDir)
	if len(languages) > 0 {
		t.Errorf("Case 1: Expected empty array, but got %v", languages)
	}

	// Case 3: No .idea directory
	err = os.Remove(ideaDir)
	if err != nil {
		t.Fatal(err)
	}
	languages = readIdeaDir(tempDir)
	if len(languages) > 0 {
		t.Errorf("Case 1: Expected empty array, but got %v", languages)
	}
}

func TestWriteConfig(t *testing.T) {
	// Create a temporary directory to use as the path
	dir := os.TempDir()
	dir = filepath.Join(dir, "writeConfig")
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(dir)

	// Create a sample qodana.yaml file to write
	filename := "qodana.yaml"
	path := filepath.Join(dir, filename)
	q := &QodanaYaml{Version: "1.0"}
	if err := q.writeConfig(path); err != nil {
		t.Fatalf("failed to write qodana.yaml file: %v", err)
	}

	// Read the contents of the file and check that it matches the expected YAML
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read qodana.yaml file: %v", err)
	}
	expected := "version: \"1.0\"\n"
	if string(data) != expected {
		t.Errorf("file contents do not match expected YAML: %q", string(data))
	}
}

func Test_setDeviceID(t *testing.T) {
	err := os.Unsetenv(qodanaRemoteUrl)
	if err != nil {
		return
	}

	tc := "Empty"
	err = os.Setenv("SALT", "")
	if err != nil {
		t.Fatal(err)
	}
	err = os.Setenv("DEVICEID", "")
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := filepath.Join(os.TempDir(), "entrypoint")
	err = os.MkdirAll(tmpDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.Chdir(cwd)
		if err != nil {
			t.Fatal(err)
		}
	}()
	actualDeviceIdSalt := getDeviceIdSalt()
	if _, err := os.Stat(filepath.Join(tmpDir, ".git", "config")); !os.IsNotExist(err) {
		t.Errorf("Case: %s: /tmp/entrypoint/.git/config got created, when it should not", tc)
	}
	expectedDeviceIdSalt := []string{
		"200820300000000-0000-0000-0000-000000000000",
		"0229f593f62e84ad29a64cebb6a9b861",
	}

	if !reflect.DeepEqual(actualDeviceIdSalt, expectedDeviceIdSalt) {
		t.Errorf("Case: %s: deviceIdSalt got %v, expected %v", tc, actualDeviceIdSalt, expectedDeviceIdSalt)
	}

	tc = "FromGit"
	_, err = exec.Command("git", "init").Output()
	if err != nil {
		t.Fatal(err)
	}
	_, err = exec.Command("git", "remote", "add", "origin", "ssh://git@git/repo").Output()
	if err != nil {
		t.Fatal(err)
	}

	actualDeviceIdSalt = getDeviceIdSalt()
	expectedDeviceIdSalt = []string{
		"200820300000000-a294-0dd1-57f5-9f44b322ff64",
		"e5c8900956f0df2f18f827245f47f04a",
	}

	if !reflect.DeepEqual(actualDeviceIdSalt, expectedDeviceIdSalt) {
		t.Errorf("Case: %s: deviceIdSalt got %v, expected %v", tc, actualDeviceIdSalt, expectedDeviceIdSalt)
	}

	tc = "FromQodanaRemoteUrlEnv"
	err = os.Setenv("QODANA_REMOTE_URL", "ssh://git@git/repo")
	if err != nil {
		t.Fatal(err)
	}
	actualDeviceIdSalt = getDeviceIdSalt()
	expectedDeviceIdSalt = []string{
		"200820300000000-a294-0dd1-57f5-9f44b322ff64",
		"e5c8900956f0df2f18f827245f47f04a",
	}
	if !reflect.DeepEqual(actualDeviceIdSalt, expectedDeviceIdSalt) {
		t.Errorf("Case: %s: deviceIdSalt got %v, expected %v", tc, actualDeviceIdSalt, expectedDeviceIdSalt)
	}

	tc = "FromEnv"
	err = os.Setenv("SALT", "salt")
	if err != nil {
		t.Fatal(err)
	}
	err = os.Setenv("DEVICEID", "device")
	if err != nil {
		t.Fatal(err)
	}
	actualDeviceIdSalt = getDeviceIdSalt()
	expectedDeviceIdSalt = []string{
		"device",
		"salt",
	}

	if !reflect.DeepEqual(actualDeviceIdSalt, expectedDeviceIdSalt) {
		t.Errorf("Case: %s: deviceIdSalt got %v, expected %v", tc, actualDeviceIdSalt, expectedDeviceIdSalt)
	}
}

func Test_isProcess(t *testing.T) {
	if isProcess("non-existing_process") {
		t.Fatal("Found non-existing process")
	}
	var cmd *exec.Cmd
	var cmdString string
	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "windows" {
		cmd = exec.Command("ping", "-n", "3", "127.0.0.1")
		cmdString = "ping -n 3 127.0.0.1"
	} else {
		cmd = exec.Command("ping", "-c", "3", "127.0.0.1")
		cmdString = "ping -c 3 127.0.0.1"
	}
	go func() {
		err := cmd.Run()
		if err != nil {
			t.Errorf("Failed to start test process: %v", err)
			return
		}
	}()
	time.Sleep(time.Second)
	if !isProcess(cmdString) {
		t.Errorf("Test process was not found by isProcess")
	}
	time.Sleep(time.Second * 6)
	if isProcess(cmdString) {
		t.Errorf("Test process was found by isProcess after it should have finished")
	}
}

func Test_runCmd(t *testing.T) {
	if //goland:noinspection ALL
	runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		for _, tc := range []struct {
			name string
			cmd  []string
			res  int
		}{
			{"true", []string{"true"}, 0},
			{"false", []string{"false"}, 1},
			{"exit 255", []string{"sh", "-c", "exit 255"}, 255},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got := RunCmd("", tc.cmd...)
				if got != tc.res {
					t.Errorf("runCmd: %v, Got: %v, Expected: %v", tc.cmd, got, tc.res)
				}
			})
		}
	}
}

func Test_createUser(t *testing.T) {
	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "windows" {
		return
	}

	err := os.Setenv(qodanaDockerEnv, "true")
	if err != nil {
		t.Fatal(err)
	}
	tc := "User"
	err = os.MkdirAll("/tmp/entrypoint", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("/tmp/entrypoint/passwd", []byte("root:x:0:0:root:/root:/bin/bash\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	createUser("/tmp/entrypoint/passwd")
	res := fmt.Sprintf("root:x:0:0:root:/root:/bin/bash\nidea:x:%d:%d:idea:/root:/bin/bash", os.Getuid(), os.Getgid())
	if os.Getuid() == 0 {
		res = "root:x:0:0:root:/root:/bin/bash\n"
	}
	got, err := os.ReadFile("/tmp/entrypoint/passwd")
	if err != nil || string(got) != res {
		t.Errorf("Case: %s: Got: %s\n Expected: %v", tc, got, res)
	}

	tc = "UserAgain"
	createUser("/tmp/entrypoint/passwd")
	got, err = os.ReadFile("/tmp/entrypoint/passwd")
	if err != nil || string(got) != res {
		t.Errorf("Case: %s: Got: %s\n Expected: %v", tc, got, res)
	}

	err = os.RemoveAll("/tmp/entrypoint")
	if err != nil {
		t.Fatal(err)
	}
}

func Test_syncIdeaCache(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "entrypoint")
	tc := "NotExist"
	syncIdeaCache(filepath.Join(tmpDir, "1"), filepath.Join(tmpDir, "2"), true)
	if _, err := os.Stat(filepath.Join(tmpDir, "2")); err == nil {
		t.Errorf("Case: %s: Folder dst created, when it should not", tc)
	}

	tc = "NoOverwrite"
	err := os.MkdirAll(filepath.Join(tmpDir, "1", ".idea", "dir1", "dir2"), os.FileMode(0o755))
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(filepath.Join(tmpDir, "2", ".idea", "dir1"), os.FileMode(0o755))
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(tmpDir, "1", ".idea", "file1"), []byte("test1"), os.FileMode(0o600))
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(tmpDir, "1", ".idea", "dir1", "file2"), []byte("test2"), os.FileMode(0o600))
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(tmpDir, "2", ".idea", "dir1", "file2"), []byte("test!"), os.FileMode(0o600))
	if err != nil {
		t.Fatal(err)
	}
	syncIdeaCache(filepath.Join(tmpDir, "1"), filepath.Join(tmpDir, "2"), false)
	if _, err := os.Stat(filepath.Join(tmpDir, "1", ".idea", "dir1", "dir2")); os.IsNotExist(err) {
		t.Errorf("Case: %s: Resulting folder .idea/dir1/dir2 not found", tc)
	}
	got, err := os.ReadFile(filepath.Join(tmpDir, "2", ".idea", "dir1", "file2"))
	if err != nil || string(got) != "test!" {
		t.Errorf("Case: %s: Got: %s\n Expected: test!", tc, got)
	}

	tc = "Overwrite"
	syncIdeaCache(filepath.Join(tmpDir, "2"), filepath.Join(tmpDir, "1"), true)
	got, err = os.ReadFile(filepath.Join(tmpDir, "1", ".idea", "dir1", "file2"))
	if err != nil || string(got) != "test!" {
		t.Errorf("Case: %s: Got: %s\n Expected: test!", tc, got)
	}

	err = os.RemoveAll(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_Bootstrap(t *testing.T) {
	opts := &QodanaOptions{}
	tmpDir := filepath.Join(os.TempDir(), "entrypoint")
	err := os.MkdirAll(tmpDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	opts.ProjectDir = tmpDir
	bootstrap("echo \"bootstrap: touch qodana.yml\" > qodana.yaml", opts.ProjectDir)
	qConfig = getQodanaYaml(tmpDir)
	bootstrap(qConfig.Bootstrap, opts.ProjectDir)
	if _, err := os.Stat(filepath.Join(opts.ProjectDir, "qodana.yaml")); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("No qodana.yml created by the bootstrap command in qodana.yaml")
	}
	err = os.RemoveAll(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
}

// TestSaveProperty saves some SARIF example file, adds a property to it, then checks that a compact version of that JSON file equals the given expected expected.
func Test_SaveProperty(t *testing.T) {
	opts := &QodanaOptions{}
	tmpDir := filepath.Join(os.TempDir(), "entrypoint")
	err := os.MkdirAll(tmpDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	opts.ProjectDir = tmpDir
	shortSarif := filepath.Join(tmpDir, "qodana-short.sarif.json")
	err = os.WriteFile(
		shortSarif,
		[]byte("{\"$schema\":\"https://raw.githubusercontent.com/schemastore/schemastore/master/src/schemas/json/sarif-2.1.0-rtm.5.json\",\"version\":\"2.1.0\",\"runs\":[{\"tool\":{\"driver\":{\"name\":\"QDRICKROLL\",\"fullName\":\"Qodana for RickRolling\",\"version\":\"223.1218.100\",\"rules\":[],\"taxa\":[],\"language\":\"en-US\",\"contents\":[\"localizedData\",\"nonLocalizedData\"],\"isComprehensive\":false},\"extensions\":[]},\"invocations\":[{\"exitCode\":0,\"toolExecutionNotifications\":[],\"executionSuccessful\":true}],\"language\":\"en-US\",\"results\":[],\"automationDetails\":{\"id\":\"project/qodana/2022-08-01\",\"guid\":\"87d2cf90-9968-4bd3-9cbc-d1b624f37fd2\",\"properties\":{\"jobUrl\":\"\",\"tags\":[\"jobUrl\"]}},\"newlineSequences\":[\"\\r\\n\",\"\\n\"],\"properties\":{\"deviceId\":\"200820300000000-0000-0000-0000-000000000001\",\"tags\":[\"deviceId\"]}}]}"),
		0o644,
	)
	if err != nil {
		t.Fatal(err)
	}
	link := "https://youtu.be/dQw4w9WgXcQ"
	err = saveSarifProperty(
		shortSarif,
		"reportUrl",
		link,
	)
	if err != nil {
		t.Fatal(err)
	}
	s, err := sarif.Open(shortSarif)
	if err != nil {
		t.Fatal(err)
	}
	if s.Runs[0].Properties["reportUrl"] != link {
		t.Fatal("reportUrl was not added correctly to qodana-short.sarif.json")
	}
	expected := []byte(`{"version":"2.1.0","$schema":"https://raw.githubusercontent.com/schemastore/schemastore/master/src/schemas/json/sarif-2.1.0-rtm.5.json","runs":[{"tool":{"driver":{"contents":["localizedData","nonLocalizedData"],"fullName":"Qodana for RickRolling","isComprehensive":false,"language":"en-US","name":"QDRICKROLL","rules":[],"version":"223.1218.100"}},"invocations":[{"executionSuccessful":true,"exitCode":0}],"results":[],"automationDetails":{"guid":"87d2cf90-9968-4bd3-9cbc-d1b624f37fd2","id":"project/qodana/2022-08-01","properties":{"jobUrl":"","tags":["jobUrl"]}},"language":"en-US","newlineSequences":["\r\n","\n"],"properties":{"deviceId":"200820300000000-0000-0000-0000-000000000001","reportUrl":"https://youtu.be/dQw4w9WgXcQ","tags":["deviceId"]}}]}`)
	content, err := os.ReadFile(shortSarif)
	if err != nil {
		t.Fatal("Error when opening file: ", err)
	}
	actual := new(bytes.Buffer)
	err = json.Compact(actual, content)
	if err != nil {
		t.Fatal("Error when compacting file: ", err)
	}
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatal("Expected: ", string(expected), " Actual: ", actual.String())
	}
	err = os.RemoveAll(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
}

//goland:noinspection ALL
func Test_WriteAppInfo(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "entrypoint")
	err := os.MkdirAll(tmpDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	prod.Version = "2022.1"
	prod.EAP = true
	prod.Build = "420.69"
	prod.Code = "QDTEST"
	prod.Name = "Qodana for Tests"
	xmlFilePath := filepath.Join(tmpDir, "QodanaAppInfo.xml")
	writeAppInfo(xmlFilePath)
	actual, err := os.ReadFile(xmlFilePath)
	if err != nil {
		t.Fatal(err)
	}
	expected := fmt.Sprintf(`<component xmlns="http://jetbrains.org/intellij/schema/application-info"
               xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
               xsi:schemaLocation="http://jetbrains.org/intellij/schema/application-info http://jetbrains.org/intellij/schema/ApplicationInfo.xsd">
      <version major="2022" minor="1" eap="true"/>
      <company name="JetBrains s.r.o." url="https://www.jetbrains.com" copyrightStart="2000"/>
      <build number="QDTEST-420.69" date="%s" />
      <names product="Qodana for Tests" fullname="Qodana for Tests"/>
      <icon svg="xxx.svg" svg-small="xxx.svg"/>
      <plugins url="https://plugins.jetbrains.com/" builtin-url="__BUILTIN_PLUGINS_URL__"/>
</component>`, getDateNow())
	assert.Equal(t, expected, string(actual))
	err = os.RemoveAll(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
}

//goland:noinspection HttpUrlsUsage
func Test_ReadAppInfo(t *testing.T) {
	tempDir := os.TempDir()
	entrypointDir := filepath.Join(tempDir, "entrypoint")
	xmlFilePath := filepath.Join(entrypointDir, "bin", "QodanaAppInfo.xml")
	err := os.MkdirAll(filepath.Dir(xmlFilePath), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(
		xmlFilePath,
		[]byte(`<component xmlns="http://jetbrains.org/intellij/schema/application-info"
			   xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
			   xsi:schemaLocation="http://jetbrains.org/intellij/schema/application-info http://jetbrains.org/intellij/schema/ApplicationInfo.xsd">
	  <version major="2022" minor="1" eap="true"/>
	  <company name="JetBrains s.r.o." url="https://www.jetbrains.com" copyrightStart="2000"/>
	  <build number="QDTEST-420.69" date="202212060511" />
	  <names product="Qodana for Tests" fullname="Qodana for Tests"/>
	  <plugins url="https://plugins.jetbrains.com/" builtin-url="__BUILTIN_PLUGINS_URL__"/>
	</component>`),
		0o644,
	)
	if err != nil {
		t.Fatal(err)
	}
	appInfoContents := readAppInfoXml(entrypointDir)
	assert.Equal(t, "2022", appInfoContents.Version.Major)
	assert.Equal(t, "1", appInfoContents.Version.Minor)
	assert.Equal(t, "true", appInfoContents.Version.Eap)
	assert.Equal(t, "QDTEST-420.69", appInfoContents.Build.Number)
	assert.Equal(t, "202212060511", appInfoContents.Build.Date)
	assert.Equal(t, "Qodana for Tests", appInfoContents.Names.Product)
	assert.Equal(t, "Qodana for Tests", appInfoContents.Names.Fullname)
	err = os.RemoveAll(entrypointDir)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_ideaExitCode(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "entrypoint")
	err := os.MkdirAll(tmpDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		c      int
		sarif  string
		result int
	}{
		{
			name:   "both exit codes are 0",
			c:      0,
			sarif:  "{\"runs\": [{\"invocations\": [{\"exitCode\": 0}]}]}",
			result: 0,
		},
		{
			name:   "idea.sh exited with 1, no SARIF exitCode",
			c:      1,
			sarif:  "{}",
			result: 1,
		},
		{
			name:   "idea.sh exited successfully, SARIF has exitCode 255",
			c:      0,
			sarif:  "{\"runs\": [{\"invocations\": [{\"exitCode\": 255}]}]}",
			result: 255,
		},
		{
			name:   "idea.sh exited with 1, takes precedence over successful SARIF exitCode",
			c:      1,
			sarif:  "{\"runs\": [{\"invocations\": [{\"exitCode\": 0}]}]}",
			result: 1,
		},
		{
			name:   "SARIF exitCode too large, gets normalized to 1",
			c:      0,
			sarif:  "{\"runs\": [{\"invocations\": [{\"exitCode\": 256}]}]}",
			result: 1,
		},
		{
			name:   "no SARIF exitCode found",
			c:      0,
			sarif:  "{\"runs\": [{\"invocations\": [{\"exitCode2\": 2}]}]}",
			result: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err = os.WriteFile(filepath.Join(tmpDir, "qodana-short.sarif.json"), []byte(tc.sarif), 0o600)
			if err != nil {
				t.Fatal(err)
			}
			got := getIdeExitCode(tmpDir, tc.c)
			if got != tc.result {
				t.Errorf("Got: %d, Expected: %d", got, tc.result)
			}
		})
	}
	err = os.RemoveAll(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetupLicense(t *testing.T) {
	prod.Code = "QDJVM"
	prod.EAP = false
	license := `{"licenseId":"VA5HGQWQH6","licenseKey":"VA5HGQWQH6","expirationDate":"2023-07-31","licensePlan":"EAP_ULTIMATE_PLUS"}`
	expectedKey := "VA5HGQWQH6"

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, license)
	}))
	defer svr.Close()
	err := os.Setenv(QodanaLicenseEndpoint, svr.URL)
	if err != nil {
		t.Fatal(err)
	}
	setupLicense("token")

	licenseKey := os.Getenv(QodanaLicense)
	if licenseKey != expectedKey {
		t.Errorf("expected key to be '%s' got '%s'", expectedKey, licenseKey)
	}

	err = os.Unsetenv(QodanaLicenseEndpoint)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Unsetenv(QodanaLicense)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetupLicenseToken(t *testing.T) {
	for _, testData := range []struct {
		name       string
		token      string
		loToken    string
		resToken   string
		sendFus    bool
		sendReport bool
	}{
		{
			name:       "no key",
			token:      "",
			loToken:    "",
			resToken:   "",
			sendFus:    true,
			sendReport: false,
		},
		{
			name:       "with token",
			token:      "a",
			loToken:    "",
			resToken:   "a",
			sendFus:    true,
			sendReport: true,
		},
		{
			name:       "with license only token",
			token:      "",
			loToken:    "b",
			resToken:   "b",
			sendFus:    false,
			sendReport: false,
		},
		{
			name:       "both tokens",
			token:      "a",
			loToken:    "b",
			resToken:   "a",
			sendFus:    true,
			sendReport: true,
		},
	} {
		t.Run(testData.name, func(t *testing.T) {
			err := os.Setenv(QodanaLicenseOnlyToken, testData.loToken)
			if err != nil {
				t.Fatal(err)
			}
			err = os.Setenv(QodanaToken, testData.token)
			if err != nil {
				t.Fatal(err)
			}
			setupLicenseToken(&QodanaOptions{})

			if cloud.Token.Token != testData.resToken {
				t.Errorf("expected token to be '%s' got '%s'", testData.resToken, cloud.Token.Token)
			}

			sendFUS := cloud.Token.IsAllowedToSendFUS()
			if sendFUS != testData.sendFus {
				t.Errorf("expected allow FUS to be '%t' got '%t'", testData.sendFus, sendFUS)
			}

			toSendReports := cloud.Token.IsAllowedToSendReports()
			if toSendReports != testData.sendReport {
				t.Errorf("expected allow send report to be '%t' got '%t'", testData.sendReport, toSendReports)
			}

			err = os.Unsetenv(QodanaLicenseOnlyToken)
			if err != nil {
				t.Fatal(err)
			}

			err = os.Unsetenv(QodanaToken)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQodanaOptions_RequiresToken(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	tests := []struct {
		name     string
		linter   string
		ide      string
		expected bool
	}{
		{
			"QDPYC docker",
			Image(QDPYC),
			"",
			false,
		},
		{
			"QDJVMC ide",
			"",
			QDJVMC,
			false,
		},
		{
			QodanaLicense,
			"",
			"",
			false,
		},
	}

	for _, tt := range tests {
		var token string
		for _, env := range []string{QodanaToken, QodanaLicenseOnlyToken, QodanaLicense} {
			if os.Getenv(env) != "" {
				token = os.Getenv(env)
				err := os.Unsetenv(env)
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		t.Run(tt.name, func(t *testing.T) {
			if tt.name == QodanaLicense {
				err := os.Setenv(QodanaLicense, "test")
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					err := os.Unsetenv(QodanaLicense)
					if err != nil {
						t.Fatal(err)
					}
				}()
			}
			o := QodanaOptions{
				Linter: tt.linter,
				Ide:    tt.ide,
			}
			result := o.RequiresToken()
			assert.Equal(t, tt.expected, result)
		})
		if token != "" {
			err := os.Setenv(QodanaToken, token)
			if err != nil {
				t.Fatal(err)
			}
			err = os.Setenv(QodanaLicenseOnlyToken, token)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func propertiesFixture(enableStats bool, additionalProperties []string) []string {
	properties := []string{
		"-Dfus.internal.reduce.initial.delay=true",
		fmt.Sprintf("-Didea.application.info.value=%s", filepath.Join(os.TempDir(), "entrypoint", "QodanaAppInfo.xml")),
		"-Didea.class.before.app=com.jetbrains.rider.protocol.EarlyBackendStarter",
		fmt.Sprintf("-Didea.config.path=%s", filepath.Join(os.TempDir(), "entrypoint")),
		fmt.Sprintf("-Didea.headless.enable.statistics=%t", enableStats),
		"-Didea.headless.statistics.device.id=FAKE",
		"-Didea.headless.statistics.max.files.to.send=5000",
		"-Didea.headless.statistics.salt=FAKE",
		fmt.Sprintf("-Didea.log.path=%s", filepath.Join(os.TempDir(), "entrypoint", "log")),
		"-Didea.parent.prefix=Rider",
		"-Didea.platform.prefix=Qodana",
		fmt.Sprintf("-Didea.plugins.path=%s", filepath.Join(os.TempDir(), "entrypoint", "plugins", "master")),
		"-Didea.qodana.thirdpartyplugins.accept=true",
		fmt.Sprintf("-Didea.system.path=%s", filepath.Join(os.TempDir(), "entrypoint", "idea", "master")),
		"-Dinspect.save.project.settings=true",
		"-Djava.awt.headless=true",
		"-Djava.net.useSystemProxies=true",
		"-Djdk.attach.allowAttachSelf=true",
		`-Djdk.http.auth.tunneling.disabledSchemes=""`,
		"-Djdk.module.illegalAccess.silent=true",
		"-Dkotlinx.coroutines.debug=off",
		"-Dqodana.automation.guid=FAKE",
		"-Didea.job.launcher.without.timeout=true",
		"-Dqodana.coverage.input=/data/coverage",
		"-Drider.collect.full.container.statistics=true",
		"-Drider.suppress.std.redirect=true",
		"-Dsun.io.useCanonCaches=false",
		"-Dsun.tools.attach.tmp.only=true",
		"-XX:+HeapDumpOnOutOfMemoryError",
		"-XX:+UseG1GC",
		"-XX:-OmitStackTraceInFastThrow",
		"-XX:CICompilerCount=2",
		"-XX:MaxJavaStackTraceDepth=10000",
		"-XX:MaxRAMPercentage=70",
		"-XX:ReservedCodeCacheSize=512m",
		"-XX:SoftRefLRUPolicyMSPerMB=50",
		fmt.Sprintf("-Xlog:gc*:%s", filepath.Join(os.TempDir(), "entrypoint", "log", "gc.log")),
		"-ea",
	}
	properties = append(properties, additionalProperties...)
	sort.Strings(properties)
	return properties
}

func Test_Properties(t *testing.T) {
	opts := &QodanaOptions{}
	tmpDir := filepath.Join(os.TempDir(), "entrypoint")
	opts.ProjectDir = tmpDir
	opts.ResultsDir = opts.ProjectDir
	opts.CacheDir = opts.ProjectDir
	opts.CoverageDir = "/data/coverage"
	opts.AnalysisId = "FAKE"

	prod.BaseScriptName = "rider"
	prod.Code = "QDNET"
	prod.Version = "main"

	err := os.Setenv(QodanaDistEnv, opts.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Setenv(QodanaConfEnv, opts.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Setenv("DEVICEID", "FAKE")
	if err != nil {
		t.Fatal(err)
	}
	err = os.Setenv("SALT", "FAKE")
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(opts.ProjectDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name          string
		cliProperties []string
		qodanaYaml    string
		isContainer   bool
		expected      []string
	}{
		{
			name:          "no overrides, just defaults and .NET project",
			cliProperties: []string{},
			qodanaYaml:    "dotnet:\n   project: project.csproj",
			isContainer:   false,
			expected:      propertiesFixture(true, []string{"-Dqodana.net.project=project.csproj"}),
		},
		{
			name:          "target frameworks set in YAML",
			cliProperties: []string{},
			qodanaYaml:    "dotnet:\n   frameworks: net5.0;net6.0",
			isContainer:   false,
			expected:      propertiesFixture(true, []string{"-Dqodana.net.targetFrameworks=net5.0;net6.0"}),
		},
		{
			name:          "target frameworks set in YAML in container",
			cliProperties: []string{},
			qodanaYaml:    "dotnet:\n   frameworks: net5.0;net6.0",
			isContainer:   true,
			expected:      propertiesFixture(true, []string{"-Dqodana.net.targetFrameworks=net5.0;net6.0"}),
		},
		{
			name:          "target frameworks not set in container",
			cliProperties: []string{},
			qodanaYaml:    "",
			isContainer:   true,
			expected:      propertiesFixture(true, []string{"-Dqodana.net.targetFrameworks=!net48;!net472;!net471;!net47;!net462;!net461;!net46;!net452;!net451;!net45;!net403;!net40;!net35;!net20;!net11"}),
		},
		{
			name:          "add one CLI property and .NET solution settings",
			cliProperties: []string{"-xa", "idea.some.custom.property=1"},
			qodanaYaml:    "dotnet:\n   solution: solution.sln\n   configuration: Release\n   platform: x64",
			isContainer:   false,
			expected: append(
				propertiesFixture(true, []string{"-Dqodana.net.solution=solution.sln", "-Dqodana.net.configuration=Release", "-Dqodana.net.platform=x64", "-Didea.some.custom.property=1"}),
				"-xa",
			),
		},
		{
			name:          "override options from CLI, YAML should be ignored",
			cliProperties: []string{"-Dfus.internal.reduce.initial.delay=false", "-Didea.application.info.value=0", "idea.headless.enable.statistics=false"},
			qodanaYaml: "" +
				"version: \"1.0\"\n" +
				"properties:\n" +
				"  fus.internal.reduce.initial.delay: true\n" +
				"  idea.application.info.value: 0\n",
			isContainer: false,
			expected: append([]string{
				"-Dfus.internal.reduce.initial.delay=false",
				"-Didea.application.info.value=0",
			}, propertiesFixture(false, []string{})[2:]...),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err = os.WriteFile(filepath.Join(opts.ProjectDir, "qodana.yml"), []byte(tc.qodanaYaml), 0o600)
			if err != nil {
				t.Fatal(err)
			}
			opts.Property = tc.cliProperties
			qConfig = getQodanaYaml(opts.ProjectDir)
			if tc.isContainer {
				err = os.Setenv(qodanaDockerEnv, "true")
				if err != nil {
					t.Fatal(err)
				}
			}
			actual := getProperties(opts, qConfig.Properties, qConfig.DotNet, []string{})
			if tc.isContainer {
				err = os.Unsetenv(qodanaDockerEnv)
				if err != nil {
					t.Fatal(err)
				}
			}
			assert.Equal(t, tc.expected, actual)
		})
	}
	err = os.RemoveAll(opts.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
}
