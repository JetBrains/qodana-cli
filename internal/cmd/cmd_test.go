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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/core"
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/internal/platform/version"
	"github.com/JetBrains/qodana-cli/internal/testutil/needs"
	cp "github.com/otiai10/copy"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// dispatchTestRoot builds a root command with stub subcommands matching what
// InitCli registers in production. *ranCmd captures the name of whichever
// stub's Run executed (empty if none did). The scan stub also exposes a -i
// shorthand flag so tests can exercise scan flag parsing without pulling in
// the real scan command's container/native machinery.
func dispatchTestRoot() (rootCmd *cobra.Command, out *bytes.Buffer, ranCmd *string) {
	rootCmd = newRootCommand()
	out = &bytes.Buffer{}
	rootCmd.SetOut(out)
	rootCmd.SetErr(out)
	captured := ""
	ranCmd = &captured
	subcommands := []string{"scan", "init", "show", "send", "pull", "view", "contributors", "cloc"}
	for _, name := range subcommands {
		n := name
		stub := &cobra.Command{
			Use: n,
			Run: func(cmd *cobra.Command, args []string) { *ranCmd = n },
		}
		if n == "scan" {
			stub.Flags().StringP("input", "i", "", "")
		}
		rootCmd.AddCommand(stub)
	}
	return
}

// TestQD14791HelpCompletionDispatch regression-locks QD-14791:
// `qodana help completion` must reach cobra's help subcommand, never scan.
//
// In the commit that added this test, the body included an explicit
// setDefaultCommandIfNeeded(...) call that demonstrated the bug (the test
// failed because scan ran). The removal commit deleted that helper and
// the call line; this test now lives as forward-going regression coverage
// against cobra-native dispatch.
func TestQD14791HelpCompletionDispatch(t *testing.T) {
	rootCmd, out, ranCmd := dispatchTestRoot()
	rootCmd.SetArgs([]string{"help", "completion"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if *ranCmd == "scan" {
		t.Fatalf("scan ran for `qodana help completion`; QD-14791 regression.\nCaptured output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "completion") {
		t.Fatalf("expected completion-related help text in output; got:\n%s", out.String())
	}
}

// TestRootDispatchCompletionBash locks in the QD-9907 prior-art path:
// `qodana completion bash` must produce a bash completion script, not a scan.
func TestRootDispatchCompletionBash(t *testing.T) {
	rootCmd, out, ranCmd := dispatchTestRoot()
	rootCmd.SetArgs([]string{"completion", "bash"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if *ranCmd == "scan" {
		t.Fatalf("scan ran for `qodana completion bash`; output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "bash completion") {
		t.Fatalf("expected bash completion script in output; got first 200 bytes:\n%s", firstNBytes(out.String(), 200))
	}
}

// TestRootDispatchUnknownSubcommand verifies that an unknown subcommand
// surfaces cobra's error rather than silently injecting scan.
func TestRootDispatchUnknownSubcommand(t *testing.T) {
	rootCmd, _, ranCmd := dispatchTestRoot()
	rootCmd.SetArgs([]string{"fizzbuzz"})
	rootCmd.SilenceUsage = true

	err := rootCmd.Execute()
	if err == nil {
		t.Fatalf("expected error for unknown subcommand, got nil; ranCmd=%q", *ranCmd)
	}
	if !strings.Contains(err.Error(), "fizzbuzz") {
		t.Fatalf("expected error to mention `fizzbuzz`; got: %v", err)
	}
	if *ranCmd != "" {
		t.Fatalf("no stub should have run for `fizzbuzz`; ran %q", *ranCmd)
	}
}

// TestRootDispatchBareInvocation verifies that bare `qodana` shows root
// help (matches the existing root Run callback's len(args)==0 branch).
func TestRootDispatchBareInvocation(t *testing.T) {
	rootCmd, out, ranCmd := dispatchTestRoot()
	rootCmd.SetArgs([]string{})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if *ranCmd != "" {
		t.Fatalf("no subcommand should run for bare `qodana`; ran %q", *ranCmd)
	}
	if !strings.Contains(out.String(), "Qodana CLI") {
		t.Fatalf("expected root help in output; got first 200 bytes:\n%s", firstNBytes(out.String(), 200))
	}
}

// TestRootDispatchShellCompletion covers cobra's hidden shell-completion
// request command (called by generated completion scripts at every Tab).
// Before scan injection was removed, `qodana __complete ""` got rewritten
// to `qodana scan __complete ""`, silently breaking shell completion.
func TestRootDispatchShellCompletion(t *testing.T) {
	rootCmd, _, ranCmd := dispatchTestRoot()
	rootCmd.SetArgs([]string{cobra.ShellCompRequestCmd, ""})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if *ranCmd == "scan" {
		t.Fatalf("scan ran for cobra shell completion request; QD-14813 regression")
	}
}

// TestRootDispatchScanFlagValueMatchingSubcommandName verifies cobra parses
// `scan -i <value>` correctly when <value> happens to equal a registered
// subcommand name. Pre-removal this was a latent foot-gun in
// isCommandRequested (it called slices.Contains over the entire arg vector,
// matching the flag value); post-removal that helper is gone and only
// cobra's flag parsing decides.
func TestRootDispatchScanFlagValueMatchingSubcommandName(t *testing.T) {
	rootCmd, _, ranCmd := dispatchTestRoot()
	rootCmd.SetArgs([]string{"scan", "-i", "scan"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if *ranCmd != "scan" {
		t.Fatalf("expected scan stub to run; ran %q", *ranCmd)
	}
	scanCmd, _, _ := rootCmd.Find([]string{"scan"})
	got, err := scanCmd.Flags().GetString("input")
	if err != nil {
		t.Fatalf("could not read --input flag: %v", err)
	}
	if got != "scan" {
		t.Fatalf("expected --input value %q, got %q", "scan", got)
	}
}

func firstNBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

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
	deprecations := []string{"fixes-strategy"}

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

	updatedQodanaYamlPath := qdyaml.GetLocalNotEffectiveQodanaYamlFullPath(projectPath, "")

	if !strings.HasSuffix(updatedQodanaYamlPath, "qodana.yml") {
		t.Fatalf("expected \"qodana.yml\" got \"%s\"", updatedQodanaYamlPath)
	}

	qodanaYaml := qdyaml.LoadQodanaYamlByFullPath(updatedQodanaYamlPath)

	if qodanaYaml.Linter != product.PythonLinter.Name {
		t.Fatalf("expected \"%s\", but got %s", product.PythonLinter.Name, qodanaYaml.Linter)
	}

	err = os.RemoveAll(projectPath)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExclusiveFixesCommand(t *testing.T) {
	needs.Need(t, needs.Docker)
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
	mapData := make(map[string]any)
	err = json.Unmarshal(output, &mapData)
	if err != nil {
		t.Fatal(err)
	}
	total := mapData["total"].(float64)
	if total <= 7 {
		t.Fatalf("expected <= 7, but got %f", total)
	}
}

func TestPullImage(t *testing.T) {
	needs.Need(t, needs.Docker)
	command := newPullCommand()
	command.SetArgs([]string{"--image", "hello-world"})

	err := command.Execute()
	if err != nil {
		t.Fatal(err)
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
	needs.Need(t, needs.Docker, needs.ContainerTests)

	version.Version = "0.1.0"
	image := "jetbrains/qodana-jvm-community:latest"

	token := os.Getenv("QODANA_LICENSE_ONLY_TOKEN")
	if token != "" {
		image = "jetbrains/qodana-dotnet:latest"
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
	command.SetArgs([]string{"-i", projectPath, "--image", image})
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// scan without configuration (use explicit --image to avoid auto-detecting an unpublished image)
	scanArgs := []string{
		"-i", projectPath,
		"-o", resultsPath,
		"--cache-dir", cachePath,
		"--image", image,
		"-v", filepath.Join(projectPath, ".idea") + ":/data/some",
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
	err = os.WriteFile(yamlFile, fmt.Appendf(nil, "image: %s", image), 0o755)
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
	command.SetArgs([]string{"-i", projectPath, "-d", "--linter", image})
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
	product.RequireNightlyAuth(t)
	log.SetLevel(log.DebugLevel)
	token := os.Getenv("QODANA_LICENSE_ONLY_TOKEN")
	if token == "" {
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
			"--repository-root", projectPath,
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

func TestCacheSync(t *testing.T) {
	product.RequireNightlyAuth(t)
	log.SetLevel(log.DebugLevel)
	token := os.Getenv("QODANA_LICENSE_ONLY_TOKEN")
	if token == "" {
		t.Skip("set your token here to run the test")
	}
	projectPath := t.TempDir()
	err := cp.Copy(filepath.Join("testdata", "synccache"), projectPath)
	if err != nil {
		t.Fatal(err)
	}

	runNativeScan(t, projectPath)
	err = os.RemoveAll(filepath.Join(projectPath, ".idea"))
	if err != nil {
		log.Errorf("Failed to remove directory: %v", err)
	}
	runNativeScan(t, projectPath)
}

func runNativeScan(t *testing.T, projectPath string) {
	out := bytes.NewBufferString("")

	command := newScanCommand()
	command.SetOut(out)
	command.SetArgs(
		[]string{
			"-i", projectPath,
			"--within-docker", "false",
			"--cache-dir", filepath.Join(projectPath, "cache"),
			"--linter", "qodana-jvm",
		},
	)
	err := command.Execute()
	if err != nil {
		t.Fatal(err)
	}
}
