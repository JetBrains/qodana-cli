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

package cmd

// Provides simple CLI tests for all supported platforms.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/v2024/platform/version"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/v2024/core"
)

func createProject(t *testing.T, name string) string {
	location := filepath.Join(os.TempDir(), ".qodana_scan_", name)
	err := os.MkdirAll(location, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(location+"/hello.py", []byte("print(\"Hello\"   )"), 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(location+"/.idea", 0o755)
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
	expected := fmt.Sprintf("qodana version %s\n", version.Version)
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

func TestDeprecatedScanFlags(t *testing.T) {
	deprecations := []string{"fixes-strategy", "stub-profile"}

	out := bytes.NewBufferString("")
	command := newScanCommand()
	command.SetOut(out)
	command.SetArgs([]string{"--help"})
	err := command.Execute()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	output := string(raw)

	for _, dep := range deprecations {
		if strings.Contains(output, dep) {
			t.Fatalf("Deprecated flag in output %s", dep)
		}
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

	filename := qdyaml.FindDefaultQodanaYaml(projectPath)

	if filename != "qodana.yml" {
		t.Fatalf("expected \"qodana.yml\" got \"%s\"", filename)
	}

	qodanaYaml := qdyaml.LoadQodanaYaml(projectPath, filename)

	if qodanaYaml.Linter != platform.Image(platform.QDPY) {
		t.Fatalf("expected \"%s\", but got %s", platform.Image(platform.QDPY), qodanaYaml.Linter)
	}

	err = os.RemoveAll(projectPath)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExclusiveFixesCommand(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		//goland:noinspection GoBoolExpressions
		if _, err := exec.LookPath("docker"); err != nil || runtime.GOOS != "linux" {
			t.Skip(err)
		}
	}
	out := bytes.NewBufferString("")
	command := newScanCommand()
	command.SetOut(out)
	command.SetArgs([]string{"--apply-fixes", "--cleanup"})
	err := command.Execute()
	if err == nil {
		t.Fatal("expected error, but got nil")
	}
}

func TestContributorsCommand(t *testing.T) {
	out := bytes.NewBufferString("")
	command := newContributorsCommand()
	command.SetOut(out)
	command.SetArgs([]string{"--days", "-1", "-o", "json"})
	err := command.Execute()
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	mapData := make(map[string]interface{})
	err = json.Unmarshal(output, &mapData)
	if err != nil {
		t.Fatal(err)
	}
	total := mapData["total"].(float64)
	if total <= 7 {
		t.Fatalf("expected <= 7, but got %f", total)
	}
}

func TestPullInNative(t *testing.T) {
	projectPath := createProject(t, "qodana_scan_python_native")
	yamlFile := filepath.Join(projectPath, "qodana.yaml")
	_ = os.WriteFile(yamlFile, []byte("ide: QDPY"), 0o755)
	out := bytes.NewBufferString("")
	command := newPullCommand()
	command.SetOut(out)
	command.SetArgs([]string{"-i", projectPath})
	err := command.Execute()
	if err != nil {
		t.Fatal(err)
	}
}

func TestAllCommandsWithContainer(t *testing.T) {
	version.Version = "0.1.0"
	linter := "registry.jetbrains.team/p/sa/containers/qodana-dotnet:latest"

	token := os.Getenv("QODANA_LICENSE_ONLY_TOKEN")
	if //goland:noinspection GoBoolExpressions
	token == "" {
		t.Skip("set your token here to run the test")
	}

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		//goland:noinspection GoBoolExpressions
		if _, err := exec.LookPath("docker"); err != nil || runtime.GOOS != "linux" {
			t.Skip(err)
		}
	}
	//_ = os.Setenv(qodanaCliContainerKeep, "true")
	//_ = os.Setenv(qodanaCliContainerName, "qodana-cli-test-new1")
	msg.DisableColor()
	core.CheckForUpdates(version.Version)
	projectPath := createProject(t, "qodana_scan_python")

	// create temp directory for cache
	cachePath := filepath.Join(os.TempDir(), "qodana_cache")
	err := os.MkdirAll(cachePath, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	resultsPath := filepath.Join(projectPath, "results")
	err = os.MkdirAll(resultsPath, 0o755)
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

	// scan without configuration
	scanArgs := []string{
		"-i", projectPath,
		"-o", resultsPath,
		"--cache-dir", cachePath,
		"-v", filepath.Join(projectPath, ".idea") + ":/data/some",
		"-e", platform.QodanaLicenseOnlyToken + "=" + os.Getenv("QODANA_LICENSE_ONLY_TOKEN"),
		"--fail-threshold", "5",
		"--print-problems",
		"--apply-fixes",
		"--property",
		"idea.headless.enable.statistics=false",
	}
	out = bytes.NewBufferString("")
	// set debug log to debug
	log.SetLevel(log.DebugLevel)
	command = newScanCommand()
	command.SetOut(out)
	command.SetArgs(scanArgs)
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// second scan with a configuration and cache
	yamlFile := filepath.Join(projectPath, "qodana.yml")
	err = os.WriteFile(yamlFile, []byte(fmt.Sprintf("linter: %s", linter)), 0o755)
	if err != nil {
		t.Fatal(err)
	}
	out = bytes.NewBufferString("")
	// set debug log to debug
	log.SetLevel(log.DebugLevel)
	command = newScanCommand()
	command.SetOut(out)
	command.SetArgs(scanArgs)
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

	// contributors
	out = bytes.NewBufferString("")
	command = newContributorsCommand()
	command.SetOut(out)
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// cloc
	out = bytes.NewBufferString("")
	command = newClocCommand()
	command.SetOut(out)
	command.SetArgs([]string{"-i", projectPath})
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// cloc
	out = bytes.NewBufferString("")
	command = newClocCommand()
	command.SetOut(out)
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
	err = os.RemoveAll(cachePath)
	if err != nil {
		t.Fatal(err)
	}
}

func TestScanWithIde(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	token := os.Getenv("QODANA_LICENSE_ONLY_TOKEN")
	if //goland:noinspection GoBoolExpressions
	token == "" {
		t.Skip("set your token here to run the test")
	}
	projectPath := ".."
	resultsPath := filepath.Join(projectPath, "results")
	err := os.MkdirAll(resultsPath, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	out := bytes.NewBufferString("")

	command := newScanCommand()
	command.SetOut(out)
	command.SetArgs(
		[]string{
			"-i", projectPath,
			"-o", resultsPath,
			"--ide", "QDGO",
			"--property",
			"idea.headless.enable.statistics=false",
		},
	)
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}
}
