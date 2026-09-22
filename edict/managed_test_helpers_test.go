// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package edict

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/JetBrains/qodana-cli/edict/managed"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
)

// The same fixture can be driven by a scripted MCP client or a real Codex run.
type managedTestProject struct {
	Checkout         distilleryTestCheckout
	ProjectDirectory string
	StateDirectory   string
	InitialRevision  string
	Store            *managed.Store
}

func prepareManagedTestProject(t *testing.T, revision, projectPath string) managedTestProject {
	t.Helper()
	requireManagedDistilleryFixture(t)
	checkout := cloneDistilleryTestRepository(t, revision)
	projectDirectory := filepath.Join(checkout.RepositoryDirectory, projectPath)
	stateDirectory := filepath.Join(projectDirectory, ".edict")
	store, err := managed.NewStore(stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return managedTestProject{
		Checkout: checkout, ProjectDirectory: projectDirectory, StateDirectory: stateDirectory, Store: store,
		InitialRevision: strings.TrimSpace(runCommand(t, checkout.RepositoryDirectory, checkout.GitBinary, "rev-parse", "HEAD")),
	}
}

type managedCodexTest struct {
	project managedTestProject
	config  CodexRunConfig
}

// Supply runtime context through the environment and host configuration. User
// prompts should describe the requested work, without teaching the skill protocol.
func prepareManagedCodex(t *testing.T, project managedTestProject) managedCodexTest {
	t.Helper()
	// Keep this fixture workflow fast without changing the model used by other
	// Codex integrations. CODEX_MODEL still allows provider-specific overrides.
	model := strings.TrimSpace(os.Getenv("CODEX_MODEL"))
	if model == "" {
		model = "gpt-5.6-terra"
	}
	t.Logf("Managed Codex model: %s", model)
	checkout := project.Checkout
	server, agents := newManagedIntegrationServer(t, project.Store, checkout.TestRoot)
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return server }, nil,
	))
	t.Cleanup(httpServer.Close)
	binary, err := ResolveCodexExecutable()
	if err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(checkout.TestRoot, "scratch")
	if err := os.MkdirAll(scratch, 0o700); err != nil {
		t.Fatal(err)
	}
	// Codex and its workers can discover writable scratch space using TMPDIR.
	t.Setenv("TMPDIR", scratch)
	home, err := PrepareCodexHome(CodexHomeConfig{
		Directory:          filepath.Join(checkout.TestRoot, "codex-home"),
		Executable:         binary,
		PermissionsProfile: "managed-edict-test",
		ReadOnlyPaths:      []string{checkout.RepositoryDirectory, project.StateDirectory},
		WritableRoots:      []string{scratch},
		Network:            CodexNetworkPermissions{Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InstallManagedSkills(filepath.Join(home, "skills")); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(home, "config.toml")
	config := mustReadFile(t, configPath)
	// Permit the batch worker's nested evidence worker. The isolated MCP server
	// can mutate only fixture state; direct filesystem writes remain sandboxed.
	config = append(config, []byte(fmt.Sprintf(`
[features]
multi_agent = true

[agents]
max_depth = 2
max_concurrent_threads_per_session = 4

[mcp_servers.edict-mcp]
url = %s
default_tools_approval_mode = "approve"
`, tomlString(httpServer.URL)))...)
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		t.Fatal(err)
	}
	assertManagedSandboxDeniesDirectWrites(t, binary, home, project.ProjectDirectory, project.StateDirectory, scratch)
	return managedCodexTest{
		project: project,
		config: CodexRunConfig{
			Executable: binary, HomeDirectory: home, WorkingDirectory: project.ProjectDirectory,
			OutputDirectory: filepath.Join(checkout.TestRoot, "trace"), Model: model,
			AgentLogger: agents,
		},
	}
}

func (codex managedCodexTest) run(t *testing.T, prompt string, timeout time.Duration) CodexRunResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	config := codex.config
	config.Prompt = prompt
	result, err := RunCodex(ctx, config)
	if err != nil || ctx.Err() != nil {
		t.Fatalf("managed Codex integration failed: %v (context: %v)\n%s",
			err, ctx.Err(), redactManagedTrace(result.CombinedOutput()))
	}
	t.Log(strings.TrimSpace(redactManagedTrace(result.LastMessage, result.CombinedOutput())))
	return result
}

// Verify execution independently of the artifacts expected by a particular test.
func (codex managedCodexTest) assertCompletedTasks(t *testing.T, result CodexRunResult, requiredSkills ...string) {
	t.Helper()
	plan := codex.project.Store.Plan()
	if plan == nil || len(plan.Tasks) == 0 || len(plan.Tasks) < len(requiredSkills) {
		t.Fatalf("manager did not persist the required pipeline: %+v", plan)
	}
	spawned := managedSpawnedAgents(t, codex.config.HomeDirectory, result.Stdout)
	if len(spawned) < len(plan.Tasks) {
		t.Errorf("only %d native subagents were spawned for %d tasks", len(spawned), len(plan.Tasks))
	}
	seenAgents, seenSkills := make(map[string]bool), make(map[string]bool)
	for _, task := range plan.Tasks {
		if task.Status != "completed" || task.AgentID == "" || seenAgents[task.AgentID] {
			t.Errorf("task lacks a completed, distinct subagent execution: %+v", task)
		}
		if !spawned[task.AgentID] {
			t.Errorf("persisted agent ID %q was not returned by native spawn_agent", task.AgentID)
		}
		seenAgents[task.AgentID], seenSkills[task.Skill] = true, true
		assertManagedPlanOnDisk(t, codex.project.StateDirectory, *plan, task.ID, "completed")
	}
	for _, skill := range requiredSkills {
		if !seenSkills[skill] {
			t.Errorf("manager did not execute %s", skill)
		}
	}
	assertManagedAgentOutput(t, codex.project.Checkout.TestRoot, plan)
	assertManagedWorkerSetup(t, codex.config.HomeDirectory, plan)
	assertManagedLifecycle(t, codex.project.Checkout.TestRoot)
}

// Every commit needs its own evidence worker beneath the single batch task.
func (codex managedCodexTest) assertSignalExtractionTasks(t *testing.T, result CodexRunResult, commits ...managedCommitExpectation) {
	t.Helper()
	codex.assertCompletedTasks(t, result, "edict-next-batch-signal-analysis", "edict-next-signal-analysis")
	plan := codex.project.Store.Plan()
	var batches, analyses []managed.Task
	for _, task := range plan.Tasks {
		switch task.Skill {
		case "edict-next-batch-signal-analysis":
			batches = append(batches, task)
		case "edict-next-signal-analysis":
			analyses = append(analyses, task)
		default:
			t.Errorf("unexpected task %s (%s) in extraction-only plan", task.ID, task.Skill)
		}
	}
	assert.Len(t, analyses, len(commits), "each selected commit must have a distinct evidence task")
	if assert.Len(t, batches, 1, "extraction must use one batch task") {
		assert.Empty(t, batches[0].ParentID, "batch analysis must be a top-level task")
		for _, task := range analyses {
			assert.Equal(t, batches[0].ID, task.ParentID, "evidence task %s must be delegated by batch analysis", task.ID)
		}
	}
	// The persisted assignment must identify the source before any results exist.
	// Match revisions, rather than model-generated descriptions, to distinguish
	// workers and catch duplicated or missing commit assignments.
	for _, commit := range commits {
		var assigned []managed.Task
		for _, task := range analyses {
			if strings.Contains(task.Prompt, "commit-"+commit.Commit[:16]) {
				assigned = append(assigned, task)
			}
		}
		if assert.Len(t, assigned, 1, "commit %s must appear in exactly one evidence task prompt", commit.Commit) {
			assert.Contains(t, assigned[0].Prompt, commit.Commit, "task prompt must identify the exact revision")
			assert.Contains(t, assigned[0].Result, commit.Commit, "worker %s must report evidence for its assigned commit", assigned[0].ID)
		}
	}
}

func assertManagedAgentOutput(t *testing.T, testRoot string, plan *managed.Plan) {
	t.Helper()
	output := string(mustReadFile(t, filepath.Join(testRoot, "log", "edict", "edict-agents.log")))
	short := string(mustReadFile(t, filepath.Join(testRoot, "log", "edict", "edict-agent-short.log")))
	unwrapped := strings.ReplaceAll(output, "\n    ", "")
	assert.Contains(t, output, "[edict_manager/-] commentary:", "main agent output must be logged")
	assert.Contains(t, short, "[edict_manager/-] commentary:", "main agent output must be kept in the short log")
	assert.NotContains(t, output, " agent=")
	for _, line := range strings.Split(output, "\n") {
		assert.LessOrEqual(t, utf8.RuneCountInString(line), 120, "agent log line must wrap: %s", line)
	}
	// A healthy integration run must not hide a failed worker behind a successful retry.
	assert.NotContains(t, output, "[unassigned/-]", "a worker exited before its task could start")
	for _, task := range plan.Tasks {
		prefix := "[" + strings.ReplaceAll(task.Skill, "edict-next-", "edict-") + "/" + task.ID[:8] + "] "
		assert.Contains(t, output, prefix+"final:", "final output missing for managed task %s", task.ID)
		assert.Contains(t, output, prefix+"mcp: Started task", "MCP activity missing for managed task %s", task.ID)
		assert.Contains(t, unwrapped, prefix+"mcp: Task prompt assigned by ", "exact prompt missing for managed task %s", task.ID)
		assert.True(t, strings.HasPrefix(task.Prompt, "$managed-"+task.Skill+"\n"), "prompt for task %s must begin with its assigned skill", task.ID)
		loggedPrompt := task.Prompt
		assert.Contains(t, unwrapped, strings.ReplaceAll(strings.ReplaceAll(loggedPrompt, "\t", "    "), "\n", ""), "complete prompt missing from log for task %s", task.ID)
		assert.Contains(t, unwrapped, prefix+"mcp: Started task; assignment fetched and skill verified")
		assert.Contains(t, output, prefix+"mcp: edict_task_get response:", "assignment response missing for task %s", task.ID)
		assert.Contains(t, output, prefix+"mcp: edict_task_finish response:", "MCP response missing for managed task %s", task.ID)
		assert.Contains(t, short, prefix+"final:", "final output missing from short log for task %s", task.ID)
		assert.Contains(t, short, prefix+"mcp: Started task", "task status missing from short log for task %s", task.ID)
		assert.NotContains(t, short, prefix+"mcp: edict_task_get response:", "short log must omit full assignments")
		assert.NotContains(t, short, prefix+"mcp: edict_task_finish response:", "short log must omit full responses")
	}
	assert.Contains(t, output, "[edict_manager/-] final:", "final manager output must be logged")
	assert.Contains(t, short, "[edict_manager/-] final:", "final manager output must be kept in the short log")
}

func assertManagedCheckoutUnchanged(t *testing.T, project managedTestProject) {
	t.Helper()
	checkout := project.Checkout
	head := strings.TrimSpace(runCommand(t, checkout.RepositoryDirectory, checkout.GitBinary, "rev-parse", "HEAD"))
	if head != project.InitialRevision {
		t.Errorf("managed agents changed checkout HEAD to %s", head)
	}
	statePath, err := filepath.Rel(checkout.RepositoryDirectory, project.StateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	allowed := "?? " + filepath.ToSlash(statePath) + "/"
	status := nonEmptyLines(runCommand(t, checkout.RepositoryDirectory, checkout.GitBinary, "status", "--short", "--untracked-files=all"))
	for _, line := range status {
		if !strings.HasPrefix(line, allowed) {
			t.Errorf("managed agents changed an unexpected fixture path: %s", line)
		}
	}
}

type managedSignalEvidence struct {
	Label    string
	Revision string
	Line     int
}

type managedCommitExpectation struct {
	Parent   string
	Commit   string
	Path     string
	Evidence []managedSignalEvidence
}

type managedCommitSignalFile struct {
	Name   string
	Signal managedCommitSignal
}

// Verify the whole inbox so missing commits, duplicates, and out-of-range
// signals cannot hide behind the correct total number of records.
func assertManagedCommitSignals(t *testing.T, project managedTestProject, expected ...managedCommitExpectation) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(project.StateDirectory, "inbox"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, commit := range expected {
		count += len(commit.Evidence)
	}
	assert.Len(t, entries, count, "total inbox signals across %d commits", len(expected))
	byCommit := make(map[string][]managedCommitSignalFile)
	for _, entry := range entries {
		var signal managedCommitSignal
		if err := json.Unmarshal(mustReadFile(t, filepath.Join(project.StateDirectory, "inbox", entry.Name())), &signal); err != nil {
			t.Fatalf("parse inbox/%s: %v", entry.Name(), err)
		}
		byCommit[signal.Source.CommitRevision] = append(byCommit[signal.Source.CommitRevision], managedCommitSignalFile{entry.Name(), signal})
	}
	for _, commit := range expected {
		assertManagedCommitSignalFiles(t, project.Checkout, commit, byCommit[commit.Commit])
		delete(byCommit, commit.Commit)
	}
	for commit, files := range byCommit {
		for _, file := range files {
			t.Errorf("inbox/%s: source.commitRevision %q is outside the expected commit selection", file.Name, commit)
		}
	}
}

func assertManagedCommitSignalFiles(t *testing.T, checkout distilleryTestCheckout, expected managedCommitExpectation, files []managedCommitSignalFile) {
	t.Helper()
	expectedDiff := runCommand(t, checkout.RepositoryDirectory, checkout.GitBinary,
		"--no-pager", "diff", "--no-color", "--no-ext-diff", "--unified=200",
		expected.Parent, expected.Commit, "--", expected.Path, expected.Path)
	assert.Len(t, files, len(expected.Evidence), "signals for commit %s", expected.Commit)
	labels := make(map[string]bool)
	for _, file := range files {
		signal := file.Signal
		artifact := "inbox/" + file.Name
		digest := sha256.Sum256([]byte(signal.IdempotencyKey))
		expectedID := "s-" + hex.EncodeToString(digest[:])[:10]
		assert.Equal(t, expectedID, signal.ID, "%s: id must match SHA-256 of idempotencyKey", artifact)
		assert.Equal(t, signal.ID+".json", file.Name, "%s: filename must match id", artifact)
		assert.Equal(t, expected.Path, signal.FileRevision.Path, "%s: fileRevision.path must be relative to the repository root", artifact)
		assert.Equal(t, "FromCommit", signal.Source.Type, "%s: source.type", artifact)
		assert.Equal(t, expected.Commit, signal.Source.CommitRevision, "%s: source.commitRevision", artifact)
		assert.Equal(t, expected.Parent, signal.Source.ParentRevision, "%s: source.parentRevision", artifact)
		assert.Equal(t, expectedDiff, signal.Source.DiffPositiveToNegative, "%s: source.diffPositiveToNegative must match the canonical Git diff", artifact)
		assert.NotEmpty(t, strings.TrimSpace(signal.Description), "%s: description", artifact)
		assert.NotEmpty(t, strings.TrimSpace(signal.IdempotencyKey), "%s: idempotencyKey", artifact)
		assert.NotEmpty(t, strings.TrimSpace(signal.Provenance.WorkItemID), "%s: provenance.workItemId", artifact)
		index := slices.IndexFunc(expected.Evidence, func(e managedSignalEvidence) bool { return e.Label == signal.Label })
		if index < 0 || labels[signal.Label] {
			t.Errorf("%s: unexpected or duplicate label %q", artifact, signal.Label)
			continue
		}
		evidence := expected.Evidence[index]
		assert.Equal(t, evidence.Revision, signal.FileRevision.Revision, "%s: fileRevision.revision for %s evidence", artifact, signal.Label)
		assert.NotEmpty(t, signal.FileRevision.ExpectedRanges, "%s: fileRevision.expectedRanges", artifact)
		for i, r := range signal.FileRevision.ExpectedRanges {
			assert.GreaterOrEqual(t, r.Start, 1, "%s: fileRevision.expectedRanges[%d].start", artifact, i)
			assert.GreaterOrEqual(t, r.End, r.Start, "%s: fileRevision.expectedRanges[%d].end", artifact, i)
		}
		if !slices.ContainsFunc(signal.FileRevision.ExpectedRanges, func(r historicalLineRange) bool {
			return r.Start <= evidence.Line && r.End >= evidence.Line
		}) {
			t.Errorf("%s: fileRevision.expectedRanges = %+v; must include changed %s line %d", artifact, signal.FileRevision.ExpectedRanges, signal.Label, evidence.Line)
		}
		labels[signal.Label] = true
	}
	for _, evidence := range expected.Evidence {
		if !labels[evidence.Label] {
			t.Errorf("commit %s: missing %s signal", expected.Commit, evidence.Label)
		}
	}
}

// Scripted clients use the same stored task instructions as real agents.
func scriptedManagedPrompt(skill string) string {
	return "$managed-" + skill + "\nRead /skills/managed-" + skill + "/SKILL.md. Process the assigned fixture evidence."
}
