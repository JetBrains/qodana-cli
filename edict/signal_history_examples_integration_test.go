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
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	historicalSignalPath       = "income/avoid-thread-sleep-in-tests.json"
	historicalExamplePath      = "src/test/java/com/mycompany/app/AppTest.java"
	historicalPositiveRevision = "38de7f9ef8879533dbdc76237b1bf732d0421e36"
	historicalNegativeRevision = "1033043161d5df7fb7006309af1b4b21af1bacb9"
	originatingRevision        = "fedd984a1a5ab941bbcce0a5f14b01415d01918e"
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

	checkout := cloneDistilleryTestRepository(t, distilleryTestFixtureRevision)
	testRoot := checkout.TestRoot
	testRepository := checkout.RepositoryDirectory
	gitBinary := checkout.GitBinary
	requireFile(t, filepath.Join(testRepository, filepath.FromSlash(historicalSignalPath)))

	codexBinary, err := ResolveCodexExecutable()
	if err != nil {
		t.Fatal(err)
	}

	if err := InstallSkill(filepath.Join(testRepository, ".codex", "skills"), "edict-signal-history-examples"); err != nil {
		t.Fatal(err)
	}
	codexHome, err := PrepareCodexHome(CodexHomeConfig{
		Directory:          filepath.Join(testRoot, "codex-home"),
		Executable:         codexBinary,
		PermissionsProfile: "qodana-skill-test",
		WritableRoots:      []string{testRepository},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	result, err := RunCodex(ctx, CodexRunConfig{
		Executable:       codexBinary,
		HomeDirectory:    codexHome,
		WorkingDirectory: testRepository,
		OutputDirectory:  testRoot,
		Model:            CodexModelFromEnvironment(),
		Prompt: `$edict-signal-history-examples

Enrich every InspectionSpecification JSON file in the income directory from this repository history.
Follow the skill exactly and do not edit files outside income.`,
	})
	output := result.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("Codex skill timed out: %v\n%s", ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("Codex skill failed: %v\n%s", err, output)
	}
	t.Log(strings.TrimSpace(result.LastMessage))

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

func nonEmptyLines(value string) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	return slices.DeleteFunc(lines, func(line string) bool { return line == "" })
}
