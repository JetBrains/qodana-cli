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
	checkout := project.Checkout
	server := newManagedIntegrationServer(t, project.Store, checkout.TestRoot)
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
max_concurrent_threads_per_session = 3

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
			OutputDirectory: filepath.Join(checkout.TestRoot, "trace"), Model: CodexModelFromEnvironment(),
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

// Artifact verification has no dependency on Codex or a particular fixture commit.
func assertManagedCommitSignals(t *testing.T, project managedTestProject, expected managedCommitExpectation) {
	t.Helper()
	checkout := project.Checkout
	expectedDiff := runCommand(t, checkout.RepositoryDirectory, checkout.GitBinary,
		"--no-pager", "diff", "--no-color", "--no-ext-diff", "--unified=200",
		expected.Parent, expected.Commit, "--", expected.Path, expected.Path)
	entries, err := os.ReadDir(filepath.Join(project.StateDirectory, "inbox"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(expected.Evidence) {
		t.Fatalf("expected %d inbox signals, got %d", len(expected.Evidence), len(entries))
	}
	labels := make(map[string]bool)
	for _, entry := range entries {
		var signal managedCommitSignal
		if err := json.Unmarshal(mustReadFile(t, filepath.Join(project.StateDirectory, "inbox", entry.Name())), &signal); err != nil {
			t.Fatalf("parse inbox/%s: %v", entry.Name(), err)
		}
		artifact := "inbox/" + entry.Name()
		digest := sha256.Sum256([]byte(signal.IdempotencyKey))
		expectedID := "s-" + hex.EncodeToString(digest[:])[:10]
		assert.Equal(t, expectedID, signal.ID, "%s: id must match SHA-256 of idempotencyKey", artifact)
		assert.Equal(t, signal.ID+".json", entry.Name(), "%s: filename must match id", artifact)
		assert.Equal(t, expected.Path, signal.FileRevision.Path, "%s: fileRevision.path must be relative to the repository root", artifact)
		assert.Equal(t, "FromCommit", signal.Source.Type, "%s: source.type", artifact)
		assert.Equal(t, expected.Commit, signal.Source.CommitRevision, "%s: source.commitRevision", artifact)
		assert.Equal(t, expected.Parent, signal.Source.ParentRevision, "%s: source.parentRevision", artifact)
		assert.Equal(t, expectedDiff, signal.Source.DiffPositiveToNegative, "%s: source.diffPositiveToNegative must match the canonical Git diff", artifact)
		assert.NotEmpty(t, strings.TrimSpace(signal.Description), "%s: description", artifact)
		assert.NotEmpty(t, strings.TrimSpace(signal.IdempotencyKey), "%s: idempotencyKey", artifact)
		assert.NotEmpty(t, strings.TrimSpace(signal.Provenance["workItemId"]), "%s: provenance.workItemId", artifact)
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
			t.Errorf("missing %s signal", evidence.Label)
		}
	}
}
