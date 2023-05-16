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

package cmd

// Provides simple CLI tests for all supported platforms.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/JetBrains/qodana-cli/core"
)

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

func TestAllCommands(t *testing.T) {
	linter := "registry.jetbrains.team/p/sa/containers/qodana-python:latest"

	if os.Getenv("GITHUB_ACTIONS") == "true" {
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

	err = os.RemoveAll(resultsPath)
	if err != nil {
		t.Fatal(err)
	}
	err = os.RemoveAll(projectPath)
	if err != nil {
		t.Fatal(err)
	}
}
