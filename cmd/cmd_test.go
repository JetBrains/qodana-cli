/*
 * Copyright 2021-2022 JetBrains s.r.o.
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

package cmd

// Provides simple CLI tests for all supported platforms.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/JetBrains/qodana-cli/core"
)

var testOptions = &core.QodanaOptions{
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

func isGitHubAction() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

func createProject(t *testing.T, name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	location := filepath.Join(home, ".qodana_scan", name)
	err = os.MkdirAll(location, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(location+"/hello.py", []byte("print(\"Hello\")"), 0o755)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

// TestVersion verifies that the version command returns the correct version
func TestVersion(t *testing.T) {
	b := bytes.NewBufferString("")
	command := newRootCommand()
	command.SetOut(b)
	command.SetArgs([]string{"-v"})
	err := command.Execute()
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(b)
	if err != nil {
		t.Fatal(err)
	}
	expected := fmt.Sprintf("qodana version %s\n", core.Version)
	actual := string(out)
	if expected != actual {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}

// TestHelp verifies that the help text is returned when running the tool with the flag or without it.
func TestHelp(t *testing.T) {
	out := bytes.NewBufferString("")
	command := newRootCommand()
	command.SetOut(out)
	command.SetArgs([]string{"-h"})
	err := command.Execute()
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	expected := string(output)

	out = bytes.NewBufferString("")
	command = newRootCommand()
	command.SetOut(out)
	command.SetArgs([]string{})
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}
	output, err = io.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	actual := string(output)

	if expected != actual {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}

func TestInitCommand(t *testing.T) {
	projectPath := createProject(t, "qodana_init")
	err := os.WriteFile(projectPath+"/qodana.yml", []byte("version: 1.0"), 0o755)
	if err != nil {
		t.Fatal(err)
	}
	out := bytes.NewBufferString("")
	command := newInitCommand()
	command.SetOut(out)
	command.SetArgs([]string{"-i", projectPath})
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}

	filename := core.FindQodanaYaml(projectPath)

	if filename != "qodana.yml" {
		t.Fatalf("expected \"qodana.yml\" got \"%s\"", filename)
	}

	qodanaYaml := core.LoadQodanaYaml(projectPath, filename)

	if qodanaYaml.Linter != core.QDPY {
		t.Fatalf("expected \"%s\", but got %s", core.QDPY, qodanaYaml.Linter)
	}

	err = os.RemoveAll(projectPath)
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidCloudUrl(t *testing.T) {
	resultsPath := createProject(t, "qodana_cloud_url")
	sarifPath := resultsPath + "/" + core.QodanaReportUrlFile
	err := os.WriteFile(
		sarifPath,
		[]byte(`https://youtu.be/dQw4w9WgXcQ`), 0o644,
	)
	if err != nil {
		t.Fatal(err)
	}
	actual := core.GetReportUrl(resultsPath)
	expected := "https://youtu.be/dQw4w9WgXcQ"
	if actual != expected {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}

func TestInvalidCloudUrl(t *testing.T) {
	projectPath := createProject(t, "qodana_cloud_invalid_url")
	sarifPath := projectPath + "/" + core.QodanaShortSarifName
	err := os.WriteFile(
		sarifPath,
		[]byte(`{.0-]}`),
		0o644,
	)
	if err != nil {
		t.Fatal(err)
	}
	actual := core.GetReportUrl(projectPath)
	expected := ""
	if actual != expected {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
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
	actual := strings.Join(core.GetCmdOptions(testOptions), " ")
	if expected != actual {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}

func TestScanFlags_Script(t *testing.T) {
	testOptions := &core.QodanaOptions{
		Script: "custom-script:parameters",
	}
	expected := []string{
		"--script",
		"custom-script:parameters",
	}
	actual := core.GetCmdOptions(testOptions)
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}

func TestDirLanguagesExcluded(t *testing.T) {
	expected := []string{"Go", "Shell"}
	actual, err := core.RecognizeDirLanguages("../")
	if err != nil {
		return
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}

func Test_ExtractEnvironmentVariables(t *testing.T) {
	revisionExpected := "1234567890abcdef1234567890abcdef12345678"
	branchExpected := "main"

	if isGitHubAction() {
		t.Skip("Not running on GitHub Actions")
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
				core.QodanaEnv:       "user-defined",
				core.QodanaJobUrl:    "https://qodana.jetbrains.com/never-gonna-give-you-up",
				core.QodanaRemoteUrl: "https://qodana.jetbrains.com/never-gonna-give-you-up",
				core.QodanaBranch:    branchExpected,
				core.QodanaRevision:  revisionExpected,
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
			qodanaEnvExpected:       fmt.Sprintf("gitlab:%s", core.Version),
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
			qodanaEnvExpected:       fmt.Sprintf("jenkins:%s", core.Version),
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
			qodanaEnvExpected:       fmt.Sprintf("github-actions:%s", core.Version),
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
			qodanaEnvExpected:       fmt.Sprintf("circleci:%s", core.Version),
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
			qodanaEnvExpected:       fmt.Sprintf("azure-pipelines:%s", core.Version),
			qodanaJobUrlExpected:    "https://dev.azure.com/jetbrains/sa/_build/results?buildId=123456789",
			qodanaRemoteUrlExpected: "https://dev.azure.com/jetbrains/sa/entrypoint.git",
		},
	} {
		t.Run(tc.ci, func(t *testing.T) {
			opts := &core.QodanaOptions{}
			for k, v := range tc.variables {
				err := os.Setenv(k, v)
				if err != nil {
					t.Fatal(err)
				}
				opts.Setenv(k, v)
			}

			core.ExtractQodanaEnvironment(opts)
			currentQodanaEnv := opts.Getenv(core.QodanaEnv)
			if currentQodanaEnv != tc.qodanaEnvExpected {
				t.Errorf("Expected %s, got %s", tc.qodanaEnvExpected, currentQodanaEnv)
			}
			if !strings.HasPrefix(currentQodanaEnv, "cli:") {
				if opts.Getenv(core.QodanaJobUrl) != tc.qodanaJobUrlExpected {
					t.Errorf("Expected %s, got %s", tc.qodanaJobUrlExpected, opts.Getenv(core.QodanaJobUrl))
				}
				if opts.Getenv(core.QodanaRemoteUrl) != tc.qodanaRemoteUrlExpected {
					t.Errorf("Expected %s, got %s", tc.qodanaRemoteUrlExpected, opts.Getenv(core.QodanaRemoteUrl))
				}
				if opts.Getenv(core.QodanaRevision) != revisionExpected {
					t.Errorf("Expected %s, got %s", revisionExpected, opts.Getenv(core.QodanaRevision))
				}
				if opts.Getenv(core.QodanaBranch) != branchExpected {
					t.Errorf("Expected %s, got %s", branchExpected, opts.Getenv(core.QodanaBranch))
				}
			}
			for _, k := range []string{core.QodanaJobUrl, core.QodanaEnv, core.QodanaRemoteUrl, core.QodanaRevision, core.QodanaBranch} {
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

func TestAllCommands(t *testing.T) {
	linter := "registry.jetbrains.team/p/sa/containers/qodana-python:latest"

	if isGitHubAction() {
		//goland:noinspection GoBoolExpressions
		if _, err := exec.LookPath("docker"); err != nil || runtime.GOOS != "linux" {
			t.Skip(err)
		}
	}
	//_ = os.Setenv(qodanaCliContainerKeep, "true")
	//_ = os.Setenv(qodanaCliContainerName, "qodana-cli-test-new1")
	core.DisableColor()
	core.CheckForUpdates("0.1.0")
	projectPath := createProject(t, "qodana_scan")
	resultsPath := filepath.Join(projectPath, "results")
	err := os.MkdirAll(resultsPath, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	// pull
	out := bytes.NewBufferString("")
	command := newPullCommand()
	command.SetOut(out)
	command.SetArgs([]string{"-i", projectPath, "-l", linter})
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// scan
	out = bytes.NewBufferString("")
	// set debug log to debug
	log.SetLevel(log.DebugLevel)
	command = newScanCommand()
	command.SetOut(out)
	command.SetArgs([]string{
		"-i", projectPath,
		"-o", resultsPath,
		"--fail-threshold", "5",
		"--print-problems",
		"--clear-cache",
		"-l", linter,
	})
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// view
	out = bytes.NewBufferString("")
	command = newViewCommand()
	command.SetOut(out)
	command.SetArgs([]string{"-f", filepath.Join(resultsPath, "qodana.sarif.json")})
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// show
	out = bytes.NewBufferString("")
	command = newShowCommand()
	command.SetOut(out)
	command.SetArgs([]string{"-i", projectPath, "-d", "-l", linter})
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// init after project analysis with .idea inside
	out = bytes.NewBufferString("")
	command = newInitCommand()
	command.SetOut(out)
	command.SetArgs([]string{"-i", projectPath})
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}

	err = os.RemoveAll(resultsPath)
	if err != nil {
		t.Fatal(err)
	}
	err = os.RemoveAll(projectPath)
	if err != nil {
		t.Fatal(err)
	}
}
