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

package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	log "github.com/sirupsen/logrus"
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
	Prod.Home = string(os.PathSeparator) + "opt" + string(os.PathSeparator) + "idea"
	Prod.IdeScript = filepath.Join(Prod.Home, "bin", "idea.sh")
	err := os.Unsetenv(platform.QodanaDockerEnv)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		opts *platform.QodanaOptions
		res  []string
	}{
		{
			name: "typical set up",
			opts: &platform.QodanaOptions{ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, Linter: "jetbrains/qodana-jvm-community:latest", SourceDirectory: "./src", DisableSanity: true, RunPromo: "true", Baseline: "qodana.sarif.json", BaselineIncludeAbsent: true, SaveReport: true, ShowReport: true, Port: 8888, Property: []string{"foo.baz=bar", "foo.bar=baz"}, Script: "default", FailThreshold: "0", AnalysisId: "id", Env: []string{"A=B"}, Volumes: []string{"/tmp/foo:/tmp/foo"}, User: "1001:1001", PrintProblems: true, ProfileName: "Default", ApplyFixes: true},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--save-report", "--source-directory", "./src", "--disable-sanity", "--profile-name", "Default", "--run-promo", "true", "--baseline", "qodana.sarif.json", "--baseline-include-absent", "--fail-threshold", "0", "--fixes-strategy", "apply", "--analysis-id", "id", "--property=foo.baz=bar", "--property=foo.bar=baz", projectDir, resultsDir},
		},
		{
			name: "arguments with spaces, no properties for local runs",
			opts: &platform.QodanaOptions{ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, ProfileName: "separated words", Property: []string{"qodana.format=SARIF_AND_PROJECT_STRUCTURE", "qodana.variable.format=JSON"}, Ide: Prod.Home},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--profile-name", "\"separated words\"", projectDir, resultsDir},
		},
		{
			name: "deprecated --fixes-strategy=apply",
			opts: &platform.QodanaOptions{ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, FixesStrategy: "apply"},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--fixes-strategy", "apply", projectDir, resultsDir},
		},
		{
			name: "deprecated --fixes-strategy=cleanup",
			opts: &platform.QodanaOptions{ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, FixesStrategy: "cleanup"},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--fixes-strategy", "cleanup", projectDir, resultsDir},
		},
		{
			name: "--fixes-strategy=apply for new versions",
			opts: &platform.QodanaOptions{ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, FixesStrategy: "apply", Ide: "/opt/idea/233"},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--apply-fixes", projectDir, resultsDir},
		},
		{
			name: "--fixes-strategy=cleanup for new versions",
			opts: &platform.QodanaOptions{ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, FixesStrategy: "cleanup", Ide: "/opt/idea/233"},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--cleanup", projectDir, resultsDir},
		},
		{
			name: "--stub-profile ignored",
			opts: &platform.QodanaOptions{StubProfile: "ignored", ProjectDir: projectDir, CacheDir: cacheDir, ResultsDir: resultsDir, FixesStrategy: "cleanup", Ide: "/opt/idea/233"},
			res:  []string{filepath.FromSlash("/opt/idea/bin/idea.sh"), "inspect", "qodana", "--cleanup", projectDir, resultsDir},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.opts.Ide == "/opt/idea/233" {
				Prod.Version = "2023.3"
			} else {
				Prod.Version = "2023.2"
			}

			args := getIdeRunCommand(&QodanaOptions{tc.opts})
			assert.Equal(t, tc.res, args)
		})
	}
}

func TestScanFlags_Script(t *testing.T) {
	testOptions := &QodanaOptions{
		&platform.QodanaOptions{
			Script: "custom-script:parameters",
		},
	}
	expected := []string{
		"--script",
		"custom-script:parameters",
	}
	actual := GetIdeArgs(testOptions)
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}

func TestLegacyFixStrategies(t *testing.T) {
	cases := []struct {
		name     string
		options  *platform.QodanaOptions
		expected []string
	}{
		{
			name: "apply fixes for a container",
			options: &platform.QodanaOptions{
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
			options: &platform.QodanaOptions{
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
			options: &platform.QodanaOptions{
				ApplyFixes: true,
				Ide:        "QDPHP",
			},
			expected: []string{
				"--apply-fixes",
			},
		},
		{
			name: "fixes for unavailable IDE",
			options: &platform.QodanaOptions{
				Cleanup: true,
				Ide:     "QDNET",
			},
			expected: []string{},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.options.Ide == "QDPHP" {
				Prod.Version = "2023.3"
			} else {
				Prod.Version = "2023.2"
			}

			actual := GetIdeArgs(&QodanaOptions{tt.options})
			if !reflect.DeepEqual(tt.expected, actual) {
				t.Fatalf("expected \"%s\" got \"%s\"", tt.expected, actual)
			}
		})
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
	q := &platform.QodanaYaml{Version: "1.0"}
	if err := q.WriteConfig(path); err != nil {
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
	err := os.Unsetenv(platform.QodanaRemoteUrl)
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
	tmpDir := filepath.Join(os.TempDir(), "deviceID")
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
	actualDeviceIdSalt := platform.GetDeviceIdSalt()
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

	actualDeviceIdSalt = platform.GetDeviceIdSalt()
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
	actualDeviceIdSalt = platform.GetDeviceIdSalt()
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
	actualDeviceIdSalt = platform.GetDeviceIdSalt()
	expectedDeviceIdSalt = []string{
		"device",
		"salt",
	}

	if !reflect.DeepEqual(actualDeviceIdSalt, expectedDeviceIdSalt) {
		t.Errorf("Case: %s: deviceIdSalt got %v, expected %v", tc, actualDeviceIdSalt, expectedDeviceIdSalt)
	}

	err = os.RemoveAll(tmpDir)
	if err != nil {
		return
	}
}

func Test_isProcess(t *testing.T) {
	if isProcess("non-existing_process") {
		t.Fatal("Found non-existing process")
	}
	var cmd *exec.Cmd
	var cmdString string
	//goland:noinspection GoBoolExpressions
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

func Test_createUser(t *testing.T) {
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS == "windows" {
		return
	}

	err := os.Setenv(platform.QodanaDockerEnv, "true")
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
	tmpDir := filepath.Join(os.TempDir(), "cache")
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
	opts := &platform.QodanaOptions{}
	tmpDir := filepath.Join(os.TempDir(), "bootstrap")
	err := os.MkdirAll(tmpDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	opts.ProjectDir = tmpDir
	platform.Bootstrap("echo 'bootstrap: touch qodana.yml' > qodana.yaml", opts.ProjectDir)
	config := platform.GetQodanaYamlOrDefault(tmpDir)
	platform.Bootstrap(config.Bootstrap, opts.ProjectDir)
	if _, err := os.Stat(filepath.Join(opts.ProjectDir, "qodana.yaml")); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("No qodana.yml created by the bootstrap command in qodana.yaml")
	}
	err = os.RemoveAll(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
}

// TestSaveProperty saves some SARIF example file, adds a property to it, then checks that a compact version of that JSON file equals the given expected.
func Test_SaveProperty(t *testing.T) {
	opts := &platform.QodanaOptions{}
	tmpDir := filepath.Join(os.TempDir(), "sarif")
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

func Test_ReadAppInfo(t *testing.T) {
	tempDir := os.TempDir()
	entrypointDir := filepath.Join(tempDir, "appinfo")
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
	tmpDir := filepath.Join(os.TempDir(), "exitcode")
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
	Prod.Code = "QDJVM"
	Prod.EAP = false
	license := `{"licenseId":"VA5HGQWQH6","licenseKey":"VA5HGQWQH6","expirationDate":"2023-07-31","licensePlan":"EAP_ULTIMATE_PLUS","projectIdHash":"hash"}`
	expectedKey := "VA5HGQWQH6"
	expectedHash := "hash"

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, license)
	}))
	defer svr.Close()
	SetupLicenseAndProjectHash(&cloud.QdApiEndpoints{LintersApiUrl: svr.URL}, "token")

	licenseKey := os.Getenv(platform.QodanaLicense)
	if licenseKey != expectedKey {
		t.Errorf("expected key to be '%s' got '%s'", expectedKey, licenseKey)
	}
	projectIdHash := os.Getenv(platform.QodanaProjectIdHash)
	if projectIdHash != expectedHash {
		t.Errorf("expected projectIdHash to be '%s' got '%s'", expectedHash, projectIdHash)
	}

	err := os.Unsetenv(platform.QodanaLicense)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Unsetenv(platform.QodanaProjectIdHash)
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
			err := os.Setenv(platform.QodanaLicenseOnlyToken, testData.loToken)
			if err != nil {
				t.Fatal(err)
			}
			err = os.Setenv(platform.QodanaToken, testData.token)
			if err != nil {
				t.Fatal(err)
			}
			cloud.SetupLicenseToken(testData.token)

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

			err = os.Unsetenv(platform.QodanaLicenseOnlyToken)
			if err != nil {
				t.Fatal(err)
			}

			err = os.Unsetenv(platform.QodanaToken)
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
			platform.QodanaToken,
			"",
			"",
			true,
		},
		{
			platform.QodanaLicense,
			"",
			"",
			false,
		},
		{
			"QDPYC docker",
			platform.Image(platform.QDPYC),
			"",
			false,
		},
		{
			"QDJVMC ide",
			"",
			platform.QDJVMC,
			false,
		},
	}

	for _, tt := range tests {
		var token string
		for _, env := range []string{platform.QodanaToken, platform.QodanaLicenseOnlyToken, platform.QodanaLicense} {
			if os.Getenv(env) != "" {
				token = os.Getenv(env)
				err := os.Unsetenv(env)
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		t.Run(tt.name, func(t *testing.T) {
			if tt.name == platform.QodanaToken {
				err := os.Setenv(platform.QodanaToken, "test")
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					err := os.Unsetenv(platform.QodanaToken)
					if err != nil {
						t.Fatal(err)
					}
				}()
			} else if tt.name == platform.QodanaLicense {
				err := os.Setenv(platform.QodanaLicense, "test")
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					err := os.Unsetenv(platform.QodanaLicense)
					if err != nil {
						t.Fatal(err)
					}
				}()
			}
			o := &QodanaOptions{
				&platform.QodanaOptions{
					Linter: tt.linter,
					Ide:    tt.ide,
				},
			}
			result := o.RequiresToken(Prod.EAP || Prod.IsCommunity())
			assert.Equal(t, tt.expected, result)
		})
		if token != "" {
			err := os.Setenv(platform.QodanaToken, token)
			if err != nil {
				t.Fatal(err)
			}
			err = os.Setenv(platform.QodanaLicenseOnlyToken, token)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func propertiesFixture(enableStats bool, additionalProperties []string) []string {
	properties := []string{
		"-Dfus.internal.reduce.initial.delay=true",
		"-Dide.warmup.use.predicates=false",
		"-Dvcs.log.index.enable=false",
		fmt.Sprintf("-Didea.application.info.value=%s", filepath.Join(Prod.IdeBin(), "QodanaAppInfo.xml")),
		"-Didea.class.before.app=com.jetbrains.rider.protocol.EarlyBackendStarter",
		fmt.Sprintf("-Didea.config.path=%s", filepath.Join(os.TempDir(), "entrypoint")),
		fmt.Sprintf("-Didea.headless.enable.statistics=%t", enableStats),
		"-Didea.headless.statistics.device.id=FAKE",
		"-Didea.headless.statistics.max.files.to.send=5000",
		"-Didea.headless.statistics.salt=FAKE",
		fmt.Sprintf("-Didea.log.path=%s", filepath.Join(os.TempDir(), "entrypoint", "log")),
		"-Didea.parent.prefix=Rider",
		"-Didea.platform.prefix=Qodana",
		fmt.Sprintf("-Didea.plugins.path=%s", filepath.Join(os.TempDir(), "entrypoint", "plugins", "233")),
		"-Didea.qodana.thirdpartyplugins.accept=true",
		fmt.Sprintf("-Didea.system.path=%s", filepath.Join(os.TempDir(), "entrypoint", "idea", "233")),
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
		"-Dqodana.recommended.profile.resource=qodana-dotnet.recommended.yaml",
		"-Dqodana.starter.profile.resource=qodana-dotnet.starter.yaml",
		"-Drider.collect.full.container.statistics=true",
		"-Drider.suppress.std.redirect=true",
		"-Dscanning.in.smart.mode=false",
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
	opts := &QodanaOptions{&platform.QodanaOptions{}}
	tmpDir := filepath.Join(os.TempDir(), "entrypoint")
	opts.ProjectDir = tmpDir
	opts.ResultsDir = opts.ProjectDir
	opts.CacheDir = opts.ProjectDir
	opts.CoverageDir = "/data/coverage"
	opts.AnalysisId = "FAKE"

	Prod.BaseScriptName = "rider"
	Prod.Code = "QDNET"
	Prod.Version = "2023.3"

	err := os.Setenv(platform.QodanaDistEnv, opts.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Setenv(platform.QodanaConfEnv, opts.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Setenv(platform.QodanaDockerEnv, "true")
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
			expected:      propertiesFixture(true, []string{"-Dqodana.net.project=project.csproj", "-Dqodana.net.targetFrameworks=!net48;!net472;!net471;!net47;!net462;!net461;!net46;!net452;!net451;!net45;!net403;!net40;!net35;!net20;!net11"}),
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
			cliProperties: []string{"-Dfus.internal.reduce.initial.delay=false", "-Dide.warmup.use.predicates=true", "-Didea.application.info.value=0", "idea.headless.enable.statistics=false"},
			qodanaYaml: "" +
				"version: \"1.0\"\n" +
				"properties:\n" +
				"  fus.internal.reduce.initial.delay: true\n" +
				"  idea.application.info.value: 0\n",
			isContainer: false,
			expected: append([]string{
				"-Dfus.internal.reduce.initial.delay=false",
				"-Dide.warmup.use.predicates=true",
				"-Didea.application.info.value=0",
			}, propertiesFixture(false, []string{})[3:]...),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err = os.WriteFile(filepath.Join(opts.ProjectDir, "qodana.yml"), []byte(tc.qodanaYaml), 0o600)
			if err != nil {
				t.Fatal(err)
			}
			opts.Property = tc.cliProperties
			qConfig := platform.GetQodanaYamlOrDefault(opts.ProjectDir)
			if tc.isContainer {
				err = os.Setenv(platform.QodanaDockerEnv, "true")
				if err != nil {
					t.Fatal(err)
				}
			}
			actual := GetProperties(opts, qConfig.Properties, qConfig.DotNet, []string{})
			if tc.isContainer {
				err = os.Unsetenv(platform.QodanaDockerEnv)
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
