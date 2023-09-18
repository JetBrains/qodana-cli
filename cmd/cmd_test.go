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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	log "github.com/sirupsen/logrus"

	"github.com/JetBrains/qodana-cli/core"
)

func createProject(t *testing.T, name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	location := filepath.Join(home, ".qodana_scan_", name)
	err = os.MkdirAll(location, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(location+"/hello.py", []byte("print(\"Hello\"   )"), 0o755)
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

	if qodanaYaml.Linter != core.Image(core.QDPYC) {
		t.Fatalf("expected \"%s\", but got %s", core.Image(core.QDPYC), qodanaYaml.Linter)
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
	if total != 10 {
		t.Fatalf("expected total 10, but got %f", total)
	}
}

func TestAllCommandsWithContainer(t *testing.T) {
	linter := "jetbrains/qodana-python-community:2023.2"

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
	projectPath := createProject(t, "qodana_scan_python")
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

	// scan with a container
	out = bytes.NewBufferString("")
	// set debug log to debug
	log.SetLevel(log.DebugLevel)
	command = newScanCommand()
	command.SetOut(out)
	command.SetArgs([]string{
		"-i", projectPath,
		"-o", resultsPath,
		"--cache-dir", filepath.Join(projectPath, "cache"),
		"--fail-threshold", "5",
		"--print-problems",
		"--apply-fixes",
		"-l", linter,
		"--property",
		"idea.headless.enable.statistics=false",
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
}

func TestScanWithIde(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	ide := "QDPY"
	token := os.Getenv("TESTS_QODANA_TOKEN")
	if //goland:noinspection GoBoolExpressions
	token == "" {
		t.Skip("set your token here to run the test")
	}
	projectPath := createProject(t, "qodana_scan_python")
	resultsPath := filepath.Join(projectPath, "results")
	err := os.MkdirAll(resultsPath, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	out := bytes.NewBufferString("")

	command := newScanCommand()
	command.SetOut(out)
	command.SetArgs([]string{
		"-i", projectPath,
		"-o", resultsPath,
		"--ide", ide,
		"--property",
		"idea.headless.enable.statistics=false",
	})
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
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
	opts := &core.QodanaOptions{}
	tmpDir := filepath.Join(os.TempDir(), "entrypoint")
	opts.ProjectDir = tmpDir
	opts.ResultsDir = opts.ProjectDir
	opts.CacheDir = opts.ProjectDir
	opts.CoverageDir = "/data/coverage"
	opts.AnalysisId = "FAKE"

	core.Prod.BaseScriptName = "rider"
	core.Prod.Code = "QDNET"
	core.Prod.Version = "main"

	err := os.Setenv(core.QodanaDistEnv, opts.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Setenv(core.QodanaConfEnv, opts.ProjectDir)
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
		expected      []string
	}{
		{
			name:          "no overrides, just defaults and .NET project",
			cliProperties: []string{},
			qodanaYaml:    "dotnet:\n   project: project.csproj",
			expected:      propertiesFixture(true, []string{"-Dqodana.net.project=project.csproj"}),
		},
		{
			name:          "add one CLI property and .NET solution settings",
			cliProperties: []string{"-xa", "idea.some.custom.property=1"},
			qodanaYaml:    "dotnet:\n   solution: solution.sln\n   configuration: Release\n   platform: x64",
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
			core.Config = core.GetQodanaYaml(opts.ProjectDir)
			actual := core.GetProperties(opts, core.Config.Properties, core.Config.DotNet, []string{})
			assert.Equal(t, tc.expected, actual)
		})
	}
	err = os.RemoveAll(opts.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
}
