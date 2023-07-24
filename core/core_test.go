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
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/owenrumney/go-sarif/v2/sarif"
	"github.com/stretchr/testify/assert"
)

var testOptions = &QodanaOptions{
	ResultsDir:            "./results",
	CacheDir:              "./cache",
	ProjectDir:            "./project",
	Linter:                "jetbrains/qodana-jvm-community:latest",
	SourceDirectory:       "./src",
	DisableSanity:         true,
	RunPromo:              "true",
	Baseline:              "qodana.sarif.json",
	BaselineIncludeAbsent: true,
	SaveReport:            true,
	ShowReport:            true,
	Port:                  8888,
	Property:              []string{"foo.baz=bar", "foo.bar=baz"},
	Script:                "default",
	FailThreshold:         "0",
	AnalysisId:            "id",
	Env:                   []string{"A=B"},
	Volumes:               []string{"/tmp/foo:/tmp/foo"},
	User:                  "1001:1001",
	PrintProblems:         true,
	ProfileName:           "Default",
}

// TestScanFlags verify that the option struct is converted to the wanted Qodana Docker options.
func TestScanFlags(t *testing.T) {
	expected := strings.Join([]string{
		"--save-report",
		"--source-directory",
		"./src",
		"--disable-sanity",
		"--profile-name",
		"Default",
		"--run-promo true",
		"--baseline",
		"qodana.sarif.json",
		"--baseline-include-absent",
		"--fail-threshold",
		"0",
		"--analysis-id",
		"id",
		"--property=foo.baz=bar",
		"--property=foo.bar=baz",
	}, " ")
	actual := strings.Join(getIdeArgs(testOptions), " ")
	if expected != actual {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
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
			actual := GetReportUrl(resultsPath)
			if actual != tc.expectedUrl {
				t.Fatalf("expected \"%s\" got \"%s\"", tc.expectedUrl, actual)
			}
		})
	}
}

func Test_ExtractEnvironmentVariables(t *testing.T) {
	revisionExpected := "1234567890abcdef1234567890abcdef12345678"
	branchExpected := "refs/heads/main"

	if os.Getenv("GITHUB_ACTIONS") == "true" {
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

	for _, tc := range []struct {
		ci                      string
		variables               map[string]string
		qodanaJobUrlExpected    string
		qodanaEnvExpected       string
		qodanaRemoteUrlExpected string
	}{
		{
			ci:                "no CI detected",
			variables:         map[string]string{},
			qodanaEnvExpected: "cli:dev",
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
			qodanaEnvExpected:       "user-defined",
			qodanaRemoteUrlExpected: "https://qodana.jetbrains.com/never-gonna-give-you-up",
			qodanaJobUrlExpected:    "https://qodana.jetbrains.com/never-gonna-give-you-up",
		},
		{
			ci: "GitLab",
			variables: map[string]string{
				"CI_JOB_URL":        "https://gitlab.jetbrains.com/never-gonna-give-you-up",
				"CI_COMMIT_BRANCH":  branchExpected,
				"CI_COMMIT_SHA":     revisionExpected,
				"CI_REPOSITORY_URL": "https://gitlab.jetbrains.com/sa/entrypoint.git",
			},
			qodanaEnvExpected:       fmt.Sprintf("gitlab:%s", Version),
			qodanaRemoteUrlExpected: "https://gitlab.jetbrains.com/sa/entrypoint.git",
			qodanaJobUrlExpected:    "https://gitlab.jetbrains.com/never-gonna-give-you-up",
		},
		{
			ci: "Jenkins",
			variables: map[string]string{
				"BUILD_URL":        "https://jenkins.jetbrains.com/never-gonna-give-you-up",
				"GIT_LOCAL_BRANCH": branchExpected,
				"GIT_COMMIT":       revisionExpected,
				"GIT_URL":          "https://git.jetbrains.com/sa/entrypoint.git",
			},
			qodanaEnvExpected:       fmt.Sprintf("jenkins:%s", Version),
			qodanaJobUrlExpected:    "https://jenkins.jetbrains.com/never-gonna-give-you-up",
			qodanaRemoteUrlExpected: "https://git.jetbrains.com/sa/entrypoint.git",
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
			qodanaEnvExpected:       fmt.Sprintf("github-actions:%s", Version),
			qodanaJobUrlExpected:    "https://github.jetbrains.com/sa/entrypoint/actions/runs/123456789",
			qodanaRemoteUrlExpected: "https://github.jetbrains.com/sa/entrypoint.git",
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
			qodanaEnvExpected:       fmt.Sprintf("github-actions:%s", Version),
			qodanaJobUrlExpected:    "https://github.jetbrains.com/sa/entrypoint/actions/runs/123456789",
			qodanaRemoteUrlExpected: "https://github.jetbrains.com/sa/entrypoint.git",
		},
		{
			ci: "CircleCI",
			variables: map[string]string{
				"CIRCLE_BUILD_URL":      "https://circleci.jetbrains.com/never-gonna-give-you-up",
				"CIRCLE_SHA1":           revisionExpected,
				"CIRCLE_BRANCH":         branchExpected,
				"CIRCLE_REPOSITORY_URL": "https://circleci.jetbrains.com/sa/entrypoint.git",
			},
			qodanaEnvExpected:       fmt.Sprintf("circleci:%s", Version),
			qodanaJobUrlExpected:    "https://circleci.jetbrains.com/never-gonna-give-you-up",
			qodanaRemoteUrlExpected: "https://circleci.jetbrains.com/sa/entrypoint.git",
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
			qodanaEnvExpected:       fmt.Sprintf("azure-pipelines:%s", Version),
			qodanaJobUrlExpected:    "https://dev.azure.com/jetbrains/sa/_build/results?buildId=123456789",
			qodanaRemoteUrlExpected: "https://dev.azure.com/jetbrains/sa/entrypoint.git",
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

			extractQodanaEnvironment(opts)
			currentQodanaEnv := opts.getenv(qodanaEnv)
			if currentQodanaEnv != tc.qodanaEnvExpected {
				t.Errorf("Expected %s, got %s", tc.qodanaEnvExpected, currentQodanaEnv)
			}
			if !strings.HasPrefix(currentQodanaEnv, "cli:") {
				if opts.getenv(qodanaJobUrl) != tc.qodanaJobUrlExpected {
					t.Errorf("Expected %s, got %s", tc.qodanaJobUrlExpected, opts.getenv(qodanaJobUrl))
				}
				if opts.getenv(qodanaRemoteUrl) != tc.qodanaRemoteUrlExpected {
					t.Errorf("Expected %s, got %s", tc.qodanaRemoteUrlExpected, opts.getenv(qodanaRemoteUrl))
				}
				if opts.getenv(qodanaRevision) != revisionExpected {
					t.Errorf("Expected %s, got %s", revisionExpected, opts.getenv(qodanaRevision))
				}
				if opts.getenv(qodanaBranch) != branchExpected {
					t.Errorf("Expected %s, got %s", branchExpected, opts.getenv(qodanaBranch))
				}
			}
			for _, k := range []string{qodanaJobUrl, qodanaEnv, qodanaRemoteUrl, qodanaRevision, qodanaBranch} {
				err := os.Unsetenv(k)
				if err != nil {
					t.Fatal(err)
				}
			}
			for k := range tc.variables {
				err := os.Unsetenv(k)
				if err != nil {
					t.Fatal(err)
				}
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

func TestParseCommits(t *testing.T) {
	gitLogOutput := []string{
		"me@me.com||me||0e64c1b093d07762ffd28c0faec75a55f67c2260||2023-05-05 16:11:38 +0200",
		"me@me.com||me||0e64c1b093d07762ffd28c0faec75a55f67c2260||2023-05-05 16:11:38 +0200",
	}

	commits := parseCommits(gitLogOutput, true)

	expectedCount := 2
	if len(commits) != expectedCount {
		t.Fatalf("Expected %d commits, got %d", expectedCount, len(commits))
	}

	expectedSha256 := "0e64c1b093d07762ffd28c0faec75a55f67c2260"
	if commits[0].Sha256 != expectedSha256 {
		t.Errorf("Expected SHA256 %s, got %s", expectedSha256, commits[0].Sha256)
	}

	expectedDate := "2023-05-05 16:11:38 +0200"
	if commits[1].Date != expectedDate {
		t.Errorf("Expected date %s, got %s", expectedDate, commits[1].Date)
	}
}

func TestGetContributors(t *testing.T) {
	contributors := GetContributors([]string{"."}, -1, false)
	if len(contributors) == 0 {
		t.Error("Expected at least one contributor or you need to update the test repo")
	}
	found := false
	for _, c := range contributors {
		if c.Author.Username == "dependabot[bot]" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected dependabot[bot] contributor")
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
	if runtime.GOOS == "windows" {
		cmd = exec.Command("ping", "-n", "5", "127.0.0.1")
		cmdString = "ping -n 5 127.0.0.1"
	} else {
		cmd = exec.Command("ping", "-c", "5", "127.0.0.1")
		cmdString = "ping -c 5 127.0.0.1"
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
	if runtime.GOOS == "windows" {
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
	Config = GetQodanaYaml(tmpDir)
	bootstrap(Config.Bootstrap, opts.ProjectDir)
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

func Test_WriteAppInfo(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "entrypoint")
	err := os.MkdirAll(tmpDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	Prod.Version = "2022.1"
	Prod.EAP = true
	Prod.Build = "420.69"
	Prod.Code = "QDTEST"
	Prod.Name = "Qodana for Tests"
	xmlFilePath := filepath.Join(tmpDir, "QodanaAppInfo.xml")
	writeAppInfo(xmlFilePath)
	actual, err := os.ReadFile(xmlFilePath)
	if err != nil {
		t.Fatal(err)
	}
	expected := `<component xmlns="http://jetbrains.org/intellij/schema/application-info"
               xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
               xsi:schemaLocation="http://jetbrains.org/intellij/schema/application-info http://jetbrains.org/intellij/schema/ApplicationInfo.xsd">
      <version major="2022" minor="1" eap="true"/>
      <company name="JetBrains s.r.o." url="https://www.jetbrains.com" copyrightStart="2000"/>
      <build number="QDTEST-420.69" date="202212060511" />
      <names product="Qodana for Tests" fullname="Qodana for Tests"/>
      <icon svg="xxx.svg" svg-small="xxx.svg"/>
      <plugins url="https://plugins.jetbrains.com/" builtin-url="__BUILTIN_PLUGINS_URL__"/>
</component>`
	assert.Equal(t, expected, string(actual))
	err = os.RemoveAll(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
}

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

func TestGetPublisherArgs(t *testing.T) {
	// Set up test options
	opts := &QodanaOptions{
		AnalysisId: "test-analysis-id",
		ProjectDir: "/path/to/project",
		ResultsDir: "/path/to/results",
		ReportDir:  "/path/to/report",
	}

	// Set up test environment variables
	os.Setenv(qodanaToolEnv, "test-tool")
	os.Setenv(qodanaEndpoint, "test-endpoint")

	// Call the function being tested
	publisherArgs := getPublisherArgs("test-publisher.jar", opts, "test-token", "test-endpoint")

	// Assert that the expected arguments are present
	expectedArgs := []string{
		Prod.jbrJava(),
		"-jar",
		"test-publisher.jar",
		"--analysis-id", "test-analysis-id",
		"--sources-path", "/path/to/project",
		"--report-path", filepath.FromSlash("/path/to/report/results"),
		"--token", "test-token",
		"--tool", "test-tool",
		"--endpoint", "test-endpoint",
	}
	if !stringSlicesEqual(publisherArgs, expectedArgs) {
		t.Errorf("getPublisherArgs returned incorrect arguments: got %v, expected %v", publisherArgs, expectedArgs)
	}
}

// Helper function to compare two string slices
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFetchPublisher(t *testing.T) {

	tempDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(tempDir) // clean up

	fetchPublisher(tempDir)

	expectedPath := filepath.Join(tempDir, "publisher.jar")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("fetchPublisher() failed, expected %v to exists, got error: %v", expectedPath, err)
	}
}
