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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	historyFixtureProject      = "testHistoryFixSignals"
	historyFixtureCluster      = "avoid-thread-sleep-for-synchronization"
	historyFixtureInspectionID = "AvoidThreadSleepForSynchronization"
	historyFixtureSourcePath   = "testHistoryFixSignals/src/main/java/com/mycompany/app/PollingWaiter.java"
	historyFixtureSnapshot     = "b6d8c9ec55d646bd01e0a47b93235376d7b13779"
	historyFixtureFixRevision  = "16f44bad3587e0f95ec5ff711ba213820de9ed35"
)

type qodanaHistorySignal struct {
	ID           string                    `json:"id"`
	FileRevision qodanaHistoryFileRevision `json:"fileRevision"`
	Source       qodanaHistorySignalSource `json:"source"`
	Label        string                    `json:"label"`
	Description  string                    `json:"description"`
	Strength     string                    `json:"strength"`
}

type qodanaHistoryFileRevision struct {
	Path           string                `json:"path"`
	Revision       string                `json:"revision"`
	ExpectedRanges []historicalLineRange `json:"expectedRanges"`
}

type qodanaHistorySignalSource struct {
	Type           string `json:"type"`
	CommitRevision string `json:"commitRevision"`
	Message        string `json:"message"`
}

func TestQodanaHistoryFixSignalsSkill(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Codex and Qodana skill integration test in short mode")
	}
	ultimateRepository := requireUltimateEdictRepository(t)
	requireCodexProvider(t)

	checkout := cloneDistilleryTestRepository(t, distilleryTestFixtureRevision)
	testRoot := checkout.TestRoot
	testRepository := checkout.RepositoryDirectory
	gitBinary := checkout.GitBinary

	codexBinary, err := ResolveCodexExecutable()
	if err != nil {
		t.Fatal(err)
	}

	projectDirectory := filepath.Join(testRepository, historyFixtureProject)
	edictDirectory := filepath.Join(projectDirectory, ".edict")
	clusterPath := filepath.Join(edictDirectory, "clusters", historyFixtureCluster, "description.json")
	inspectionPath := filepath.Join(
		edictDirectory, "inspections", historyFixtureCluster+".inspection.kts",
	)
	requireFile(t, clusterPath)
	requireFile(t, inspectionPath)
	clusterBefore := mustReadFile(t, clusterPath)
	inspectionBefore := mustReadFile(t, inspectionPath)

	systemTemporaryDirectory := os.TempDir()
	signalOutputDirectory := filepath.Join(edictDirectory, "inbox")
	qodanaResultsDirectory := filepath.Join(testRoot, "qodana-results")
	if err := os.MkdirAll(qodanaResultsDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", qodanaResultsDirectory)
	userDirectory, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	bazelCacheDirectory := filepath.Join(userDirectory, "Library", "Caches", "JetBrains", "MonorepoBazel")
	requireDirectory(t, bazelCacheDirectory)
	kotlinDaemonDirectory := filepath.Join(userDirectory, "Library", "Application Support", "kotlin", "daemon")
	requireDirectory(t, kotlinDaemonDirectory)
	codexHome, err := PrepareCodexHome(CodexHomeConfig{
		Directory:          filepath.Join(testRoot, "codex-home"),
		Executable:         codexBinary,
		PermissionsProfile: "qodana-history-skill-test",
		WritableRoots: []string{
			testRepository,
			testRoot,
			ultimateRepository,
			systemTemporaryDirectory,
			bazelCacheDirectory,
			kotlinDaemonDirectory,
		},
		Network: CodexNetworkPermissions{
			Enabled:                        true,
			Mode:                           "full",
			AllowLocalBinding:              true,
			DangerouslyAllowAllUnixSockets: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	skillsDirectory := filepath.Join(codexHome, "skills")
	if err := InstallSkill(skillsDirectory, "edict-qodana-history-fix-signals"); err != nil {
		t.Fatal(err)
	}
	installLocalBazelQodanaRunnerSkill(t, skillsDirectory)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	prompt := fmt.Sprintf(`$edict-qodana-history-fix-signals
$qodana-local-bazel-runner

Run the skill completely with these inputs:
- Cluster ID: %s
- Edict directory: %s
- Date: 2026-09-13

Use the local Bazel runner skill for both Qodana analyses. The period is intentionally omitted and
must use the history skill's default. Do not edit the Edict inputs or analyzed source. Finish only
after restoring the repository to its original branch and HEAD.`, historyFixtureCluster, edictDirectory)
	result, err := RunCodex(ctx, CodexRunConfig{
		Executable:       codexBinary,
		HomeDirectory:    codexHome,
		WorkingDirectory: projectDirectory,
		OutputDirectory:  testRoot,
		Model:            CodexModelFromEnvironment(),
		Prompt:           prompt,
	})
	output := result.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("Codex skill timed out: %v\n%s", ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("Codex skill failed: %v\n%s", err, output)
	}
	t.Log(strings.TrimSpace(result.LastMessage))

	assertQodanaHistoryCommands(t, output)
	assertAbsentFixtureResult(t, qodanaResultsDirectory)
	assertHistorySignals(t, signalOutputDirectory)

	if head := strings.TrimSpace(runCommand(t, testRepository, gitBinary, "rev-parse", "HEAD")); head != distilleryTestFixtureRevision {
		t.Errorf("skill left repository at %s, expected %s", head, distilleryTestFixtureRevision)
	}
	if branch := strings.TrimSpace(runCommand(t, testRepository, gitBinary, "branch", "--show-current")); branch != "main" {
		t.Errorf("skill left repository on branch %q, expected main", branch)
	}
	if after := mustReadFile(t, clusterPath); !slices.Equal(after, clusterBefore) {
		t.Error("skill changed the supplied cluster")
	}
	if after := mustReadFile(t, inspectionPath); !slices.Equal(after, inspectionBefore) {
		t.Error("skill changed the supplied inspection")
	}
	status := nonEmptyLines(runCommand(t, testRepository, gitBinary, "status", "--short", "--untracked-files=all"))
	for _, line := range status {
		if !strings.HasPrefix(line, "?? "+historyFixtureProject+"/.edict/inbox/s-") || !strings.HasSuffix(line, ".json") {
			t.Errorf("skill modified an unexpected path: %s", line)
		}
	}
}

func assertQodanaHistoryCommands(t *testing.T, output string) {
	t.Helper()
	snapshotInvocation := false
	baselineInvocation := false
	for _, line := range nonEmptyLines(output) {
		var event struct {
			Type string `json:"type"`
			Item struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"item"`
		}
		if json.Unmarshal([]byte(line), &event) != nil || event.Type != "item.completed" ||
			event.Item.Type != "command_execution" {
			continue
		}
		if strings.Contains(event.Item.Command, "/bin/sh ./bazel.cmd run //build:qodana_for_jvm -- qodana ") &&
			!strings.Contains(event.Item.Command, "--baseline") {
			snapshotInvocation = true
		}
		if strings.Contains(event.Item.Command, "/bin/sh ./bazel.cmd run //build:qodana_for_jvm -- qodana --baseline ") &&
			strings.Contains(event.Item.Command, "--baseline-include-absent") {
			baselineInvocation = true
		}
	}
	if !snapshotInvocation {
		t.Fatal("skill did not execute the snapshot analysis with the required Qodana command")
	}
	if !baselineInvocation {
		t.Fatal("skill did not execute the HEAD analysis with the snapshot baseline and absent results")
	}
}

func assertAbsentFixtureResult(t *testing.T, resultsDirectory string) {
	t.Helper()
	absent := 0
	reports := 0
	err := filepath.WalkDir(resultsDirectory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "qodana.sarif.json" {
			return nil
		}
		reports++
		var report struct {
			Runs []struct {
				Results []struct {
					RuleID        string `json:"ruleId"`
					BaselineState string `json:"baselineState"`
				} `json:"results"`
			} `json:"runs"`
		}
		if err := json.Unmarshal(mustReadFile(t, path), &report); err != nil {
			return fmt.Errorf("parse Qodana report %s: %w", path, err)
		}
		for _, run := range report.Runs {
			for _, result := range run.Results {
				if result.RuleID == historyFixtureInspectionID && result.BaselineState == "absent" {
					absent++
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if reports < 2 {
		t.Fatalf("expected snapshot and HEAD Qodana reports, got %d", reports)
	}
	if absent != 1 {
		t.Fatalf("expected one absent fixture result, got %d", absent)
	}
}

func assertHistorySignals(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected two generated signals, got %d", len(entries))
	}

	signals := make(map[string]qodanaHistorySignal, 2)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			t.Fatalf("unexpected signal output entry %s", entry.Name())
		}
		content := mustReadFile(t, filepath.Join(directory, entry.Name()))
		var signal qodanaHistorySignal
		if err := json.Unmarshal(content, &signal); err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		if entry.Name() != signal.ID+".json" {
			t.Errorf("signal filename %s does not match id %s", entry.Name(), signal.ID)
		}
		if signal.ID != expectedHistorySignalID(signal) {
			t.Errorf("signal %s has a non-deterministic id", signal.ID)
		}
		if signal.FileRevision.Path != historyFixtureSourcePath {
			t.Errorf("signal %s has path %s", signal.ID, signal.FileRevision.Path)
		}
		if signal.Source.Type != "FromCommit" || signal.Source.CommitRevision != historyFixtureFixRevision ||
			!strings.Contains(signal.Source.Message, "explicit worker synchronization") {
			t.Errorf("signal %s has incorrect fix provenance: %+v", signal.ID, signal.Source)
		}
		if signal.Strength != "STRONG" || signal.Description == "" {
			t.Errorf("signal %s has incomplete metadata", signal.ID)
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(content, &raw); err != nil {
			t.Fatalf("parse raw %s: %v", entry.Name(), err)
		}
		if _, legacyLanguage := raw["language"]; legacyLanguage {
			t.Errorf("signal %s contains legacy language field", signal.ID)
		}
		if syntheticExampleID, ok := raw["syntheticExampleId"]; !ok || string(syntheticExampleID) != "null" {
			t.Errorf("signal %s is not an unclustered pending signal", signal.ID)
		}
		signals[signal.Label] = signal
	}

	positive, ok := signals["POSITIVE"]
	if !ok {
		t.Fatal("positive signal was not generated")
	}
	if positive.FileRevision.Revision != historyFixtureSnapshot ||
		!slices.Equal(positive.FileRevision.ExpectedRanges, []historicalLineRange{{Start: 5, End: 5}}) {
		t.Errorf("unexpected positive evidence: %+v", positive.FileRevision)
	}
	negative, ok := signals["NEGATIVE"]
	if !ok {
		t.Fatal("negative signal was not generated")
	}
	if negative.FileRevision.Revision != historyFixtureFixRevision ||
		!slices.Equal(negative.FileRevision.ExpectedRanges, []historicalLineRange{{Start: 10, End: 10}}) {
		t.Errorf("unexpected negative evidence: %+v", negative.FileRevision)
	}
}

func expectedHistorySignalID(signal qodanaHistorySignal) string {
	ranges := "null"
	if signal.FileRevision.ExpectedRanges != nil {
		parts := make([]string, 0, len(signal.FileRevision.ExpectedRanges))
		for _, lineRange := range signal.FileRevision.ExpectedRanges {
			parts = append(parts, fmt.Sprintf("%d:%d", lineRange.Start, lineRange.End))
		}
		ranges = strings.Join(parts, ",")
	}
	material := strings.Join([]string{
		historyFixtureCluster,
		signal.Label,
		signal.FileRevision.Path,
		signal.FileRevision.Revision,
		ranges,
	}, "\n")
	digest := sha256.Sum256([]byte(material))
	return "s-" + hex.EncodeToString(digest[:])[:10]
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
