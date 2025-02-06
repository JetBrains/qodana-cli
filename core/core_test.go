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
	"errors"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/JetBrains/qodana-cli/v2024/core/scan"
	startup2 "github.com/JetBrains/qodana-cli/v2024/core/startup"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
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

	"github.com/stretchr/testify/assert"
)

func TestCliArgs(t *testing.T) {
	dir, _ := os.Getwd()
	projectDir := filepath.Join(dir, "project")
	cacheDir := filepath.Join(dir, "cache")
	resultsDir := filepath.Join(dir, "results")
	preparehost.Prod.Home = string(os.PathSeparator) + "opt" + string(os.PathSeparator) + "idea"
	preparehost.Prod.IdeScript = filepath.Join(preparehost.Prod.Home, "bin", "idea.sh")
	err := os.Unsetenv(platform.QodanaDockerEnv)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name         string
		majorVersion string
		opts         *platform.QodanaOptions
		res          []string
	}{
		{
			name:         "typical set up",
			majorVersion: "2024.2",
			opts: &platform.QodanaOptions{
				ProjectDir:            projectDir,
				CacheDir:              cacheDir,
				ResultsDir:            resultsDir,
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
				ApplyFixes:            true,
			},
			res: []string{
				filepath.FromSlash("/opt/idea/bin/idea.sh"),
				"qodana",
				"--save-report",
				"--source-directory",
				"./src",
				"--disable-sanity",
				"--profile-name",
				"Default",
				"--run-promo",
				"true",
				"--baseline",
				"qodana.sarif.json",
				"--baseline-include-absent",
				"--fail-threshold",
				"0",
				"--fixes-strategy",
				"apply",
				"--analysis-id",
				"id",
				"--property=foo.baz=bar",
				"--property=foo.bar=baz",
				projectDir,
				resultsDir,
			},
		},
		{
			name:         "arguments with spaces, no properties for local runs",
			majorVersion: "2024.1",
			opts: &platform.QodanaOptions{
				ProjectDir:  projectDir,
				CacheDir:    cacheDir,
				ResultsDir:  resultsDir,
				ProfileName: "separated words",
				Property:    []string{"qodana.format=SARIF_AND_PROJECT_STRUCTURE", "qodana.variable.format=JSON"},
				Ide:         preparehost.Prod.Home,
			},
			res: []string{
				filepath.FromSlash("/opt/idea/bin/idea.sh"),
				"inspect",
				"qodana",
				"--profile-name",
				"\"separated words\"",
				projectDir,
				resultsDir,
			},
		},
		{
			name:         "deprecated --fixes-strategy=apply",
			majorVersion: "2024.2",
			opts: &platform.QodanaOptions{
				ProjectDir:    projectDir,
				CacheDir:      cacheDir,
				ResultsDir:    resultsDir,
				FixesStrategy: "apply",
			},
			res: []string{
				filepath.FromSlash("/opt/idea/bin/idea.sh"),
				"qodana",
				"--fixes-strategy",
				"apply",
				projectDir,
				resultsDir,
			},
		},
		{
			name:         "deprecated --fixes-strategy=cleanup",
			majorVersion: "2024.3",
			opts: &platform.QodanaOptions{
				ProjectDir:    projectDir,
				CacheDir:      cacheDir,
				ResultsDir:    resultsDir,
				FixesStrategy: "cleanup",
			},
			res: []string{
				filepath.FromSlash("/opt/idea/bin/idea.sh"),
				"qodana",
				"--fixes-strategy",
				"cleanup",
				projectDir,
				resultsDir,
			},
		},
		{
			name:         "--fixes-strategy=apply for new versions",
			majorVersion: "2023.3",
			opts: &platform.QodanaOptions{
				ProjectDir:    projectDir,
				CacheDir:      cacheDir,
				ResultsDir:    resultsDir,
				FixesStrategy: "apply",
				Ide:           "/opt/idea/233",
			},
			res: []string{
				filepath.FromSlash("/opt/idea/bin/idea.sh"),
				"inspect",
				"qodana",
				"--apply-fixes",
				projectDir,
				resultsDir,
			},
		},
		{
			name:         "--fixes-strategy=cleanup for new versions",
			majorVersion: "2023.3",
			opts: &platform.QodanaOptions{
				ProjectDir:    projectDir,
				CacheDir:      cacheDir,
				ResultsDir:    resultsDir,
				FixesStrategy: "cleanup",
				Ide:           "/opt/idea/233",
			},
			res: []string{
				filepath.FromSlash("/opt/idea/bin/idea.sh"),
				"inspect",
				"qodana",
				"--cleanup",
				projectDir,
				resultsDir,
			},
		},
		{
			name:         "--stub-profile ignored",
			majorVersion: "2023.3",
			opts: &platform.QodanaOptions{
				StubProfile:   "ignored",
				ProjectDir:    projectDir,
				CacheDir:      cacheDir,
				ResultsDir:    resultsDir,
				FixesStrategy: "cleanup",
				Ide:           "/opt/idea/233",
			},
			res: []string{
				filepath.FromSlash("/opt/idea/bin/idea.sh"),
				"inspect",
				"qodana",
				"--cleanup",
				projectDir,
				resultsDir,
			},
		},
	} {
		t.Run(
			tc.name, func(t *testing.T) {
				preparehost.Prod.Version = tc.majorVersion

				args := getIdeRunCommand(&QodanaOptions{tc.opts})
				assert.Equal(t, tc.res, args)
			},
		)
	}
}

func TestScanFlags_Script(t *testing.T) {
	b := scan.ContextBuilder{
		Script: "custom-script:parameters",
	}
	expected := []string{
		"--script",
		"custom-script:parameters",
	}
	actual := GetIdeArgs(b.AsContext())
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}

func TestLegacyFixStrategies(t *testing.T) {
	cases := []struct {
		name     string
		c        scan.ContextBuilder
		expected []string
	}{
		{
			name: "apply fixes for a container",
			c: scan.ContextBuilder{
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
			c: scan.ContextBuilder{
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
			c: scan.ContextBuilder{
				ApplyFixes: true,
				Ide:        "QDPHP",
			},
			expected: []string{
				"--apply-fixes",
			},
		},
		{
			name: "fixes for unavailable IDE",
			c: scan.ContextBuilder{
				Cleanup: true,
				Ide:     "QDNET",
			},
			expected: []string{},
		},
	}

	for _, tt := range cases {
		t.Run(
			tt.name, func(t *testing.T) {
				c := tt.c
				if c.Ide == "QDPHP" {
					c.Prod.Version = "2023.3"
				} else {
					c.Prod.Version = "2023.2"
				}

				actual := GetIdeArgs(c.AsContext())
				if !reflect.DeepEqual(tt.expected, actual) {
					t.Fatalf("expected \"%s\" got \"%s\"", tt.expected, actual)
				}
			},
		)
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
	q := &qdyaml.QodanaYaml{Version: "1.0"}
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
	t.Setenv("SALT", "")
	t.Setenv("DEVICEID", "")
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
	actualDeviceIdSalt := utils.GetDeviceIdSalt()
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

	actualDeviceIdSalt = utils.GetDeviceIdSalt()
	expectedDeviceIdSalt = []string{
		"200820300000000-a294-0dd1-57f5-9f44b322ff64",
		"e5c8900956f0df2f18f827245f47f04a",
	}

	if !reflect.DeepEqual(actualDeviceIdSalt, expectedDeviceIdSalt) {
		t.Errorf("Case: %s: deviceIdSalt got %v, expected %v", tc, actualDeviceIdSalt, expectedDeviceIdSalt)
	}

	tc = "FromQodanaRemoteUrlEnv"
	t.Setenv("QODANA_REMOTE_URL", "ssh://git@git/repo")
	actualDeviceIdSalt = utils.GetDeviceIdSalt()
	expectedDeviceIdSalt = []string{
		"200820300000000-a294-0dd1-57f5-9f44b322ff64",
		"e5c8900956f0df2f18f827245f47f04a",
	}
	if !reflect.DeepEqual(actualDeviceIdSalt, expectedDeviceIdSalt) {
		t.Errorf("Case: %s: deviceIdSalt got %v, expected %v", tc, actualDeviceIdSalt, expectedDeviceIdSalt)
	}

	tc = "FromEnv"
	t.Setenv("SALT", "salt")
	t.Setenv("DEVICEID", "device")
	actualDeviceIdSalt = utils.GetDeviceIdSalt()
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
	if utils.FindProcess("non-existing_process") {
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
	if !utils.FindProcess(cmdString) {
		t.Errorf("Test process was not found by isProcess")
	}
	time.Sleep(time.Second * 6)
	if utils.FindProcess(cmdString) {
		t.Errorf("Test process was found by isProcess after it should have finished")
	}
}

func Test_createUser(t *testing.T) {
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS == "windows" {
		return
	}

	t.Setenv(platform.QodanaDockerEnv, "true")
	tc := "User"
	err := os.MkdirAll("/tmp/entrypoint", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("/tmp/entrypoint/passwd", []byte("root:x:0:0:root:/root:/bin/bash\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	startup2.CreateUser("/tmp/entrypoint/passwd")
	res := fmt.Sprintf("root:x:0:0:root:/root:/bin/bash\nidea:x:%d:%d:idea:/root:/bin/bash", os.Getuid(), os.Getgid())
	if os.Getuid() == 0 {
		res = "root:x:0:0:root:/root:/bin/bash\n"
	}
	got, err := os.ReadFile("/tmp/entrypoint/passwd")
	if err != nil || string(got) != res {
		t.Errorf("Case: %s: Got: %s\n Expected: %v", tc, got, res)
	}

	tc = "UserAgain"
	startup2.CreateUser("/tmp/entrypoint/passwd")
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

	t.Run(
		"NotExist", func(t *testing.T) {
			err := startup2.SyncIdeaCache(filepath.Join(tmpDir, "1"), filepath.Join(tmpDir, "2"), true)
			if err == nil {
				t.Errorf("Expected error when source folder does not exist")
			}
			if _, err := os.Stat(filepath.Join(tmpDir, "2")); err == nil {
				t.Errorf("Folder dst created, when it should not")
			}
		},
	)

	t.Run(
		"NoOverwrite", func(t *testing.T) {
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
			err = os.WriteFile(
				filepath.Join(tmpDir, "1", ".idea", "dir1", "file2"),
				[]byte("test2"),
				os.FileMode(0o600),
			)
			if err != nil {
				t.Fatal(err)
			}
			err = os.WriteFile(
				filepath.Join(tmpDir, "2", ".idea", "dir1", "file2"),
				[]byte("test!"),
				os.FileMode(0o600),
			)
			if err != nil {
				t.Fatal(err)
			}

			err = startup2.SyncIdeaCache(filepath.Join(tmpDir, "1"), filepath.Join(tmpDir, "2"), false)
			if err != nil {
				t.Fatalf("syncIdeaCache failed: %v", err)
			}

			if _, err := os.Stat(filepath.Join(tmpDir, "1", ".idea", "dir1", "dir2")); os.IsNotExist(err) {
				t.Errorf("Resulting folder .idea/dir1/dir2 not found")
			}
			got, err := os.ReadFile(filepath.Join(tmpDir, "2", ".idea", "dir1", "file2"))
			if err != nil || string(got) != "test!" {
				t.Errorf("Got: %s\n Expected: test!", string(got))
			}
		},
	)

	t.Run(
		"Overwrite", func(t *testing.T) {
			err := startup2.SyncIdeaCache(filepath.Join(tmpDir, "2"), filepath.Join(tmpDir, "1"), true)
			if err != nil {
				t.Fatalf("syncIdeaCache failed: %v", err)
			}

			got, err := os.ReadFile(filepath.Join(tmpDir, "1", ".idea", "dir1", "file2"))
			if err != nil || string(got) != "test!" {
				t.Errorf("Got: %s\n Expected: test!", string(got))
			}
		},
	)

	t.Run(
		"HasSymlinksAndFileExistsInDst", func(t *testing.T) {
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
			err = os.WriteFile(
				filepath.Join(tmpDir, "1", ".idea", "dir1", "file2"),
				[]byte("test2"),
				os.FileMode(0o600),
			)
			if err != nil {
				t.Fatal(err)
			}
			err = os.WriteFile(
				filepath.Join(tmpDir, "2", ".idea", "dir1", "file2"),
				[]byte("test!"),
				os.FileMode(0o600),
			)
			if err != nil {
				t.Fatal(err)
			}

			err = os.Symlink(
				filepath.Join(tmpDir, "1", ".idea", "dir1", "file2"),
				filepath.Join(tmpDir, "1", ".idea", "dir1", "symlink"),
			)
			if err != nil {
				t.Fatal(err)
			}

			err = startup2.SyncIdeaCache(filepath.Join(tmpDir, "1"), filepath.Join(tmpDir, "2"), false)
			if err != nil {
				t.Fatalf("syncIdeaCache failed: %v", err)
			}

			if _, err := os.Stat(filepath.Join(tmpDir, "1", ".idea", "dir1", "dir2")); os.IsNotExist(err) {
				t.Errorf("Resulting folder .idea/dir1/dir2 not found")
			}
			got, err := os.ReadFile(filepath.Join(tmpDir, "2", ".idea", "dir1", "file2"))
			if err != nil || string(got) != "test!" {
				t.Errorf("Got: %s\n Expected: test!", string(got))
			}
		},
	)

	if err := os.RemoveAll(tmpDir); err != nil {
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
	qdenv.Bootstrap("echo 'bootstrap: touch qodana.yml' > qodana.yaml", opts.ProjectDir)
	config := qdyaml.GetQodanaYamlOrDefault(tmpDir)
	qdenv.Bootstrap(config.Bootstrap, opts.ProjectDir)
	if _, err := os.Stat(filepath.Join(opts.ProjectDir, "qodana.yaml")); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("No qodana.yml created by the bootstrap command in qodana.yaml")
	}
	err = os.RemoveAll(tmpDir)
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
		t.Run(
			tc.name, func(t *testing.T) {
				err = os.WriteFile(filepath.Join(tmpDir, "qodana-short.sarif.json"), []byte(tc.sarif), 0o600)
				if err != nil {
					t.Fatal(err)
				}
				got := getIdeExitCode(tmpDir, tc.c)
				if got != tc.result {
					t.Errorf("Got: %d, Expected: %d", got, tc.result)
				}
			},
		)
	}
	err = os.RemoveAll(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetupLicense(t *testing.T) {
	preparehost.Prod.Code = "QDJVM"
	preparehost.Prod.EAP = false
	for _, tc := range []struct {
		name            string
		license         string
		expectedKey     string
		expectedHash    string
		expectedOrgHash string
	}{
		{
			name:            "valid license",
			license:         `{"licenseId":"VA5HGQWQH6","licenseKey":"VA5HGQWQH6","expirationDate":"2023-07-31","licensePlan":"EAP_ULTIMATE_PLUS","projectIdHash":"hash","organizationIdHash":"org hash"}`,
			expectedKey:     "VA5HGQWQH6",
			expectedHash:    "hash",
			expectedOrgHash: "org hash",
		},
		{
			name:            "no organizationIdHash",
			license:         `{"licenseId":"VA5HGQWQH6","licenseKey":"VA5HGQWQH6","expirationDate":"2023-07-31","licensePlan":"EAP_ULTIMATE_PLUS","projectIdHash":"hash"}`,
			expectedKey:     "VA5HGQWQH6",
			expectedHash:    "hash",
			expectedOrgHash: "",
		},
	} {
		t.Run(
			tc.name, func(t *testing.T) {
				svr := httptest.NewServer(
					http.HandlerFunc(
						func(w http.ResponseWriter, r *http.Request) {
							_, _ = fmt.Fprint(w, tc.license)
						},
					),
				)
				defer svr.Close()
				startup2.SetupLicenseAndProjectHash(&cloud.QdApiEndpoints{LintersApiUrl: svr.URL}, "token")

				licenseKey := os.Getenv(platform.QodanaLicense)
				if licenseKey != tc.expectedKey {
					t.Errorf("expected key to be '%s' got '%s'", tc.expectedKey, licenseKey)
				}

				projectIdHash := os.Getenv(platform.QodanaProjectIdHash)
				if projectIdHash != tc.expectedHash {
					t.Errorf("expected projectIdHash to be '%s' got '%s'", tc.expectedHash, projectIdHash)
				}

				if tc.expectedOrgHash == "" {
					_, r := os.LookupEnv(platform.QodanaOrganisationIdHash)
					if r {
						t.Errorf("'%s' env shoul not be set", platform.QodanaOrganisationIdHash)
					}
				} else {
					orgIdHash := os.Getenv(platform.QodanaOrganisationIdHash)
					if orgIdHash != tc.expectedOrgHash {
						t.Errorf("expected organizationIdHash to be '%s' got '%s'", tc.expectedOrgHash, orgIdHash)
					}
				}

				err := os.Unsetenv(platform.QodanaLicense)
				if err != nil {
					t.Fatal(err)
				}

				err = os.Unsetenv(platform.QodanaProjectIdHash)
				if err != nil {
					t.Fatal(err)
				}

				err = os.Unsetenv(platform.QodanaOrganisationIdHash)
				if err != nil {
					t.Fatal(err)
				}
			},
		)
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
		t.Run(
			testData.name, func(t *testing.T) {
				t.Setenv(qdenv.QodanaLicenseOnlyToken, testData.loToken)
				t.Setenv(qdenv.QodanaToken, testData.token)
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
			},
		)
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

		t.Run(
			tt.name, func(t *testing.T) {
				if tt.name == platform.QodanaToken {
					t.Setenv(platform.QodanaToken, "test")
				} else if tt.name == platform.QodanaLicense {
					t.Setenv(platform.QodanaLicense, "test")
				}
				o := &QodanaOptions{
					&platform.QodanaOptions{
						Linter: tt.linter,
						Ide:    tt.ide,
					},
				}
				result := o.RequiresToken(preparehost.Prod.EAP || preparehost.Prod.IsCommunity())
				assert.Equal(t, tt.expected, result)
			},
		)
		if token != "" {
			t.Setenv(platform.QodanaToken, token)
			t.Setenv(platform.QodanaLicenseOnlyToken, token)
		}
	}
}

func propertiesFixture(enableStats bool, additionalProperties []string) []string {
	properties := []string{
		fmt.Sprintf("-Didea.config.path=%s", filepath.Join(os.TempDir(), "entrypoint")),
		fmt.Sprintf("-Didea.headless.enable.statistics=%t", enableStats),
		"-Didea.headless.statistics.device.id=FAKE",
		"-Didea.headless.statistics.salt=FAKE",
		"-Dqodana.automation.guid=FAKE",
		"-Dqodana.coverage.input=/data/coverage",
		fmt.Sprintf("-Didea.log.path=%s", filepath.Join(os.TempDir(), "entrypoint", "log")),
		fmt.Sprintf("-Didea.plugins.path=%s", filepath.Join(os.TempDir(), "entrypoint", "plugins", "233")),
		fmt.Sprintf("-Didea.system.path=%s", filepath.Join(os.TempDir(), "entrypoint", "idea", "233")),
		fmt.Sprintf("-Xlog:gc*:%s", filepath.Join(os.TempDir(), "entrypoint", "log", "gc.log")),
		"-XX:MaxRAMPercentage=70",
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

	preparehost.Prod.BaseScriptName = "rider"
	preparehost.Prod.Code = "QDNET"
	preparehost.Prod.Version = "2023.3"

	t.Setenv(platform.QodanaDistEnv, opts.ProjectDir)
	t.Setenv(platform.QodanaConfEnv, opts.ProjectDir)
	t.Setenv(platform.QodanaDockerEnv, "true")
	t.Setenv("DEVICEID", "FAKE")
	t.Setenv("SALT", "FAKE")
	err := os.MkdirAll(opts.ProjectDir, 0o755)
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
			expected: propertiesFixture(
				true,
				[]string{"-Dqodana.net.targetFrameworks=!net48;!net472;!net471;!net47;!net462;!net461;!net46;!net452;!net451;!net45;!net403;!net40;!net35;!net20;!net11"},
			),
		},
		{
			name:          "add one CLI property and .NET solution settings",
			cliProperties: []string{"-xa", "idea.some.custom.property=1"},
			qodanaYaml:    "dotnet:\n   solution: solution.sln\n   configuration: Release\n   platform: x64",
			isContainer:   false,
			expected: append(
				propertiesFixture(
					true,
					[]string{
						"-Dqodana.net.solution=solution.sln",
						"-Dqodana.net.configuration=Release",
						"-Dqodana.net.platform=x64",
						"-Didea.some.custom.property=1",
					},
				),
				"-xa",
			),
		},
		{
			name:          "override options from CLI, YAML should be ignored",
			cliProperties: []string{"idea.headless.enable.statistics=false"},
			qodanaYaml: "" +
				"version: \"1.0\"\n" +
				"properties:\n" +
				"  idea.headless.enable.statistics: true\n" +
				"  idea.application.info.value: 0\n",
			isContainer: false,
			expected: append(
				[]string{
					"-Didea.application.info.value=0",
				}, propertiesFixture(false, []string{})...,
			),
		},
	} {
		t.Run(
			tc.name, func(t *testing.T) {
				err = os.WriteFile(filepath.Join(opts.ProjectDir, "qodana.yml"), []byte(tc.qodanaYaml), 0o600)
				if err != nil {
					t.Fatal(err)
				}
				opts.Property = tc.cliProperties
				qConfig := qdyaml.GetQodanaYamlOrDefault(opts.ProjectDir)
				if tc.isContainer {
					t.Setenv(platform.QodanaDockerEnv, "true")
				} else {
					err := os.Unsetenv(platform.QodanaDockerEnv)
					if err != nil {
						t.Fatal(err)
					}
				}
				actual := GetScanProperties(opts, qConfig.Properties, qConfig.DotNet, []string{})
				assert.Equal(t, tc.expected, actual)
			},
		)
	}
	err = os.RemoveAll(opts.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
}
