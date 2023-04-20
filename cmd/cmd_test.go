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
	Changes:               true,
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
	projectPath := createProject(t, "qodana_cloud_url")
	sarifPath := projectPath + "/" + core.QodanaShortSarifName
	err := os.WriteFile(
		sarifPath,
		[]byte(`{"version":"2.1.0","$schema":"https://raw.githubusercontent.com/schemastore/schemastore/master/src/schemas/json/sarif-2.1.0-rtm.5.json","runs":[{"tool":{"driver":{"contents":["localizedData","nonLocalizedData"],"fullName":"Qodana for RickRolling","isComprehensive":false,"language":"en-US","name":"QDRICKROLL","rules":[],"version":"223.1218.100"}},"invocations":[{"executionSuccessful":true,"exitCode":0}],"results":[],"automationDetails":{"guid":"87d2cf90-9968-4bd3-9cbc-d1b624f37fd2","id":"project/qodana/2022-08-01","properties":{"jobUrl":"","tags":["jobUrl"]}},"language":"en-US","newlineSequences":["\r\n","\n"],"properties":{"deviceId":"200820300000000-0000-0000-0000-000000000001","reportUrl":"https://youtu.be/dQw4w9WgXcQ","tags":["deviceId"]}}]}`),
		0o644,
	)
	if err != nil {
		t.Fatal(err)
	}
	actual := core.GetReportUrl(projectPath)
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
		"--local-changes",
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

func TestAllCommands(t *testing.T) {
	linter := "jetbrains/qodana-python:latest"

	if isGitHubAction() {
		//goland:noinspection GoBoolExpressions
		if _, err := exec.LookPath("docker"); err != nil || runtime.GOOS == "windows" {
			t.Skip(err)
		}
	} else {
		_ = os.Setenv("GITHUB_SERVER_URL", "https://github.com")
		_ = os.Setenv("GITHUB_REPOSITORY", "JetBrains/qodana-cli")
		_ = os.Setenv("GITHUB_RUN_ID", "1")
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
