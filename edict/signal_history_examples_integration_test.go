/*
 * Copyright 2021-2026 JetBrains s.r.o.
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

package edict

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	defaultDistilleryTestRepository = "/Users/alexey.afanasiev/prj/examples/distillery-test"
	historicalSignalPath            = "income/avoid-thread-sleep-in-tests.json"
	historicalExamplePath           = "src/test/java/com/mycompany/app/AppTest.java"
	historicalPositiveRevision      = "38de7f9ef8879533dbdc76237b1bf732d0421e36"
	historicalNegativeRevision      = "1033043161d5df7fb7006309af1b4b21af1bacb9"
	originatingRevision             = "fedd984a1a5ab941bbcce0a5f14b01415d01918e"
	defaultCodexModel               = "gpt-5.6-sol"
)

type historicalSignal struct {
	OptionalPositiveExamples []historicalFileRevision `json:"optionalPositiveExamples"`
	OptionalNegativeExamples []historicalFileRevision `json:"optionalNegativeExamples"`
}

type historicalFileRevision struct {
	Path                  string                `json:"path"`
	Revision              string                `json:"revision"`
	ExpectedProblemRanges []historicalLineRange `json:"expectedProblemRanges"`
}

type historicalLineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

func TestSignalHistoryExamplesSkill(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Codex skill integration test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the Git instrumentation wrapper requires a POSIX shell")
	}
	requireCodexProvider(t)

	sourceRepository := os.Getenv("DISTILLERY_TEST_REPO")
	if sourceRepository == "" {
		sourceRepository = defaultDistilleryTestRepository
	}
	requireDirectory(t, filepath.Join(sourceRepository, ".git"))
	requireFile(t, filepath.Join(sourceRepository, filepath.FromSlash(historicalSignalPath)))

	gitBinary, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("git is required:", err)
	}
	codexBinary := os.Getenv("CODEX_BIN")
	if codexBinary == "" {
		codexBinary = "codex"
	}
	codexBinary, err = exec.LookPath(codexBinary)
	if err != nil {
		t.Fatal("codex is required:", err)
	}
	codexBinary, err = filepath.EvalSymlinks(codexBinary)
	if err != nil {
		t.Fatalf("resolve codex executable: %v", err)
	}
	model := os.Getenv("CODEX_MODEL")
	if model == "" {
		model = defaultCodexModel
	}

	packageDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	testRoot := filepath.Join(packageDirectory, "testtmp", t.Name())
	assertTestRoot(t, packageDirectory, testRoot)
	if err := os.RemoveAll(testRoot); err != nil {
		t.Fatalf("clear test directory: %v", err)
	}
	t.Cleanup(
		func() {
			if err := os.RemoveAll(testRoot); err != nil {
				t.Errorf("clear test directory: %v", err)
			}
		},
	)
	if err := os.MkdirAll(testRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	testRepository := filepath.Join(testRoot, "distillery-test")
	runCommand(t, packageDirectory, gitBinary, "clone", "--quiet", "--no-local", sourceRepository, testRepository)
	installHistoricalExamplesSkill(t, testRepository)
	codexHome := prepareCodexHome(t, testRoot, testRepository, codexBinary)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	lastMessagePath := filepath.Join(testRoot, "codex-last-message.txt")
	args := []string{
		"exec",
		"--dangerously-bypass-hook-trust",
		"--json",
		"--skip-git-repo-check",
		"--model", model,
		"--output-last-message", lastMessagePath,
	}
	args = append(
		args, `$edict-signal-history-examples

Enrich every InspectionSpecification JSON file in the income directory from this repository history.
Follow the skill exactly and do not edit files outside income.`,
	)

	command := exec.CommandContext(ctx, codexBinary, args...)
	command.Dir = testRepository
	command.Stdin = strings.NewReader("")
	command.Env = environmentWithOverrides(
		map[string]string{
			"CODEX_HOME": codexHome,
		},
	)
	stdoutPath := filepath.Join(testRoot, "stdout.jsonl")
	stderrPath := filepath.Join(testRoot, "stderr.log")
	stdout := createOutputFile(t, stdoutPath)
	stderr := createOutputFile(t, stderrPath)
	command.Stdout = stdout
	command.Stderr = stderr
	err = command.Run()
	if closeErr := stdout.Close(); closeErr != nil {
		t.Errorf("close Codex stdout: %v", closeErr)
	}
	if closeErr := stderr.Close(); closeErr != nil {
		t.Errorf("close Codex stderr: %v", closeErr)
	}
	output := readCodexOutput(t, stdoutPath, stderrPath)
	if ctx.Err() != nil {
		t.Fatalf("Codex skill timed out: %v\n%s", ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("Codex skill failed: %v\n%s", err, output)
	}
	t.Log(string(output))

	assertGitGrepUsed(t, output)
	signal := readHistoricalSignal(t, filepath.Join(testRepository, filepath.FromSlash(historicalSignalPath)))
	if !containsHistoricalExample(
		signal.OptionalPositiveExamples,
		historicalExamplePath,
		historicalPositiveRevision,
		[]historicalLineRange{{Start: 44, End: 44}},
	) {
		t.Fatalf(
			"historical positive example %s@%s:44 was not found",
			historicalExamplePath,
			historicalPositiveRevision,
		)
	}
	if !containsHistoricalExample(
		signal.OptionalNegativeExamples,
		historicalExamplePath,
		historicalNegativeRevision,
		nil,
	) {
		t.Fatalf("historical negative example %s@%s was not found", historicalExamplePath, historicalNegativeRevision)
	}
	if containsRevision(signal.OptionalPositiveExamples, originatingRevision) ||
		containsRevision(signal.OptionalNegativeExamples, originatingRevision) {
		t.Fatalf("originating correction %s was incorrectly added as a historical example", originatingRevision)
	}
	if len(signal.OptionalPositiveExamples) == 0 || len(signal.OptionalPositiveExamples) > 5 {
		t.Fatalf("expected 1..5 historical pairs, got %d", len(signal.OptionalPositiveExamples))
	}
	if len(signal.OptionalPositiveExamples) != len(signal.OptionalNegativeExamples) {
		t.Fatalf(
			"unpaired historical examples: %d positive, %d negative",
			len(signal.OptionalPositiveExamples),
			len(signal.OptionalNegativeExamples),
		)
	}

	allowedStatus := []string{
		" M " + historicalSignalPath,
		"?? .codex/skills/edict-signal-history-examples/SKILL.md",
	}
	status := runCommand(t, testRepository, gitBinary, "status", "--short", "--untracked-files=all")
	for _, line := range nonEmptyLines(status) {
		if !slices.Contains(allowedStatus, line) {
			t.Errorf("skill modified an unexpected path: %s", line)
		}
	}
}

func createOutputFile(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func readCodexOutput(t *testing.T, stdoutPath string, stderrPath string) string {
	t.Helper()
	stdout, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(stdout) + string(stderr)
}

func prepareCodexHome(
	t *testing.T,
	testRoot string,
	testRepository string,
	codexBinary string,
) string {
	t.Helper()
	codexHome := filepath.Join(testRoot, "codex-home")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}

	providerConfig := ""
	if strings.TrimSpace(os.Getenv("LITELLM_API_KEY")) != "" {
		providerConfig = `model_provider = "litellm"

[model_providers.litellm]
name = "LiteLLM"
base_url = "https://litellm.labs.jb.gg/openai"
env_key = "LITELLM_API_KEY"
wire_api = "responses"

`
	}
	config := fmt.Sprintf(
		`approval_policy = "never"
default_permissions = "qodana-skill-test"
model_reasoning_effort = "high"

%s[permissions.qodana-skill-test]
extends = ":read-only"

[permissions.qodana-skill-test.filesystem]
":tmpdir" = "write"
%s = "read"

[permissions.qodana-skill-test.workspace_roots]
%s = true

[permissions.qodana-skill-test.filesystem.":workspace_roots"]
"." = "write"
`, providerConfig, tomlString(codexBinary), tomlString(testRepository),
	)
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return codexHome
}

func requireCodexProvider(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("LITELLM_API_KEY")) != "" {
		connection, err := net.DialTimeout("tcp", "litellm.labs.jb.gg:443", 5*time.Second)
		if err != nil {
			t.Skipf("LiteLLM endpoint is unavailable: %v", err)
		}
		if err := connection.Close(); err != nil {
			t.Errorf("close LiteLLM preflight connection: %v", err)
		}
		return
	}
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" {
		t.Skip("LITELLM_API_KEY or OPENAI_API_KEY is required for the isolated Codex home")
	}
}

func tomlString(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

func installHistoricalExamplesSkill(t *testing.T, repository string) {
	t.Helper()
	content, err := skillsFS.ReadFile("skills/edict-signal-history-examples/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(repository, ".codex", "skills", "edict-signal-history-examples")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertGitGrepUsed(t *testing.T, output string) {
	t.Helper()
	for _, line := range nonEmptyLines(output) {
		var event struct {
			Item struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"item"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Item.Type == "command_execution" && strings.Contains(event.Item.Command, "git grep") {
			return
		}
	}
	t.Fatal("skill did not use git grep")
}

func readHistoricalSignal(t *testing.T, path string) historicalSignal {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var signal historicalSignal
	if err := json.Unmarshal(content, &signal); err != nil {
		t.Fatalf("parse enriched signal: %v", err)
	}
	return signal
}

func containsHistoricalExample(
	examples []historicalFileRevision,
	path string,
	revision string,
	ranges []historicalLineRange,
) bool {
	return slices.ContainsFunc(
		examples, func(example historicalFileRevision) bool {
			return example.Path == path && example.Revision == revision && slices.Equal(
				example.ExpectedProblemRanges,
				ranges,
			)
		},
	)
}

func containsRevision(examples []historicalFileRevision, revision string) bool {
	return slices.ContainsFunc(
		examples, func(example historicalFileRevision) bool {
			return example.Revision == revision
		},
	)
}

func environmentWithOverrides(overrides map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[key]; !replaced {
			environment = append(environment, entry)
		}
	}
	for key, value := range overrides {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func runCommand(t *testing.T, directory string, executable string, args ...string) string {
	t.Helper()
	command := exec.Command(executable, args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", executable, strings.Join(args, " "), err, output)
	}
	return string(output)
}

func requireDirectory(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("required directory %s is unavailable: %v", path, err)
	}
}

func requireFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("required file %s is unavailable: %v", path, err)
	}
}

func assertTestRoot(t *testing.T, packageDirectory string, testRoot string) {
	t.Helper()
	relative, err := filepath.Rel(packageDirectory, testRoot)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	if len(parts) < 2 || parts[0] != "testtmp" || parts[1] != t.Name() {
		t.Fatalf("refusing to clear unexpected test path %s", testRoot)
	}
}

func nonEmptyLines(value string) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	return slices.DeleteFunc(lines, func(line string) bool { return line == "" })
}
