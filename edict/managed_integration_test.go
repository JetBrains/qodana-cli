/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package edict

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JetBrains/qodana-cli/edict/managed"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This test drives real MCP requests and real Git evidence deterministically. The
// agent IDs below represent a scripted client; TestManagedEdictManagerSkill is the
// separate test that actually launches Codex and its subagents.
func TestManagedEdictDistilleryWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping distillery Git fixture integration in short mode")
	}
	project := prepareManagedTestProject(t, historyFixtureFixRevision, historyFixtureProject)
	checkout, stateDirectory := project.Checkout, project.StateDirectory
	clusterDescription := filepath.Join(stateDirectory, "clusters", historyFixtureCluster, "description.json")
	descriptionBefore := mustReadFile(t, clusterDescription)
	sourcePath := filepath.Join(checkout.RepositoryDirectory, filepath.FromSlash(historyFixtureSourcePath))
	sourceBefore := mustReadFile(t, sourcePath)

	beforeFix := runCommand(
		t,
		checkout.RepositoryDirectory,
		checkout.GitBinary,
		"show",
		historyFixtureSnapshot+":"+historyFixtureSourcePath,
	)
	afterFix := runCommand(
		t,
		checkout.RepositoryDirectory,
		checkout.GitBinary,
		"show",
		historyFixtureFixRevision+":"+historyFixtureSourcePath,
	)
	if !strings.Contains(beforeFix, "Thread.sleep(1_000)") || !strings.Contains(
		afterFix,
		"workerFinished.await(1, TimeUnit.SECONDS)",
	) {
		t.Fatal("the fixture no longer contains the expected corrective commit")
	}

	client := connectManagedTestServer(t, project.Store, checkout.TestRoot)
	ctx := context.Background()
	created := managedCall[managed.PlanCreation](
		t, ctx, client, "edict_plan_create", map[string]any{
			"request": "Extract the Thread.sleep correction and distribute its evidence to the existing cluster",
			"steps": []managed.Step{
				{Skill: "edict-next-batch-signal-analysis", Title: "Extract the corrective commit"},
				{Skill: "edict-next-run", Title: "Distribute the resulting signals"},
			},
		},
	)
	plan, managerToken := created.Plan, created.Token
	if len(plan.Tasks) != 2 {
		t.Fatalf("expected two pipeline tasks, got %+v", plan.Tasks)
	}
	batchTask, runTask := plan.Tasks[0], plan.Tasks[1]
	batch := managedCall[managed.Delegation](
		t, ctx, client, "edict_delegate", map[string]any{
			"token":      managerToken,
			"taskId":     batchTask.ID,
			"operations": []string{"inbox.write"},
			"scope":      []string{"inbox"},
		},
	)
	managedCall[managed.Plan](
		t,
		ctx,
		client,
		"edict_task_start",
		map[string]any{"token": batch.Token, "agentId": "scripted-batch-agent"},
	)
	analysisTask := managedCall[managed.Task](
		t, ctx, client, "edict_task_add", map[string]any{
			"token": batch.Token,
			"skill": "edict-next-signal-analysis",
			"title": "Read the exact before and after revisions",
		},
	)
	analysis := managedCall[managed.Delegation](
		t, ctx, client, "edict_delegate", map[string]any{
			"token": batch.Token, "taskId": analysisTask.ID, "operations": []string{}, "scope": []string{},
		},
	)
	managedCall[managed.Plan](
		t,
		ctx,
		client,
		"edict_task_start",
		map[string]any{"token": analysis.Token, "agentId": "scripted-analysis-agent"},
	)
	signals := managedFixtureSignals(t, checkout)
	writeArgs := map[string]any{
		"token":        analysis.Token,
		"path":         "inbox/" + signals[0].Path,
		"content":      signals[0].Content,
		"expectedHash": "",
	}
	managedCallDenied(t, ctx, client, "edict_state_write", writeArgs)
	writeArgs["token"] = managerToken
	managedCallDenied(t, ctx, client, "edict_state_write", writeArgs)
	writeArgs["token"] = "unmanaged-or-forged-token"
	managedCallDenied(t, ctx, client, "edict_state_write", writeArgs)
	managedCallDenied(
		t, ctx, client, "edict_task_add", map[string]any{
			"token": analysis.Token, "skill": "edict-next-generation", "title": "Disallowed call graph edge",
		},
	)
	finished := managedCall[managed.Plan](
		t, ctx, client, "edict_task_finish", map[string]any{
			"token":  analysis.Token,
			"status": "completed",
			"result": "Verified Thread.sleep before the fix and CountDownLatch.await after it",
		},
	)
	assertManagedPlanOnDisk(
		t,
		stateDirectory,
		finished,
		analysisTask.ID,
		"completed",
		managerToken,
		batch.Token,
		analysis.Token,
	)
	// Reproduce the real model failure: a project-relative path with a
	// repository-relative canonical diff must be rejected at the write boundary.
	var invalidSignal managedCommitSignal
	if err := json.Unmarshal([]byte(signals[0].Content), &invalidSignal); err != nil {
		t.Fatal(err)
	}
	invalidSignal.FileRevision.Path = strings.TrimPrefix(historyFixtureSourcePath, historyFixtureProject+"/")
	invalidContent, err := json.Marshal(invalidSignal)
	if err != nil {
		t.Fatal(err)
	}
	writeArgs["token"], writeArgs["content"] = batch.Token, string(invalidContent)
	denied, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "edict_state_write", Arguments: writeArgs})
	if err != nil {
		t.Fatal(err)
	}
	if !denied.IsError || !strings.Contains(managedResultText(denied), "fileRevision.path:") ||
		!strings.Contains(managedResultText(denied), historyFixtureSourcePath) {
		t.Fatalf("expected a field-specific repository-relative path error; got %s", managedResultText(denied))
	}
	if _, err := project.Store.Read("inbox/" + signals[0].Path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("rejected signal must not be persisted; read error: %v", err)
	}
	for i := range signals {
		signals[i] = managedCall[managed.File](
			t, ctx, client, "edict_state_write", map[string]any{
				"token":        batch.Token,
				"path":         "inbox/" + signals[i].Path,
				"content":      signals[i].Content,
				"expectedHash": "",
			},
		)
	}
	assertManagedCommitSignals(t, project, historyFixSignalExpectation())
	finished = managedCall[managed.Plan](
		t, ctx, client, "edict_task_finish", map[string]any{
			"token":  batch.Token,
			"status": "completed",
			"result": "Created paired positive and negative commit signals",
		},
	)
	assertManagedPlanOnDisk(
		t,
		stateDirectory,
		finished,
		batchTask.ID,
		"completed",
		managerToken,
		batch.Token,
		analysis.Token,
	)
	managedCallDenied(
		t, ctx, client, "edict_state_write", map[string]any{
			"token":        batch.Token,
			"path":         signals[0].Path,
			"content":      signals[0].Content,
			"expectedHash": signals[0].Hash,
		},
	)

	run := managedCall[managed.Delegation](
		t, ctx, client, "edict_delegate", map[string]any{
			"token": managerToken, "taskId": runTask.ID,
			"operations": []string{"inbox.delete", "cluster.signal.write"},
			"scope":      []string{"inbox", "clusters/" + historyFixtureCluster},
		},
	)
	managedCall[managed.Plan](
		t,
		ctx,
		client,
		"edict_task_start",
		map[string]any{"token": run.Token, "agentId": "scripted-run-agent"},
	)
	distributionTask := managedCall[managed.Task](
		t, ctx, client, "edict_task_add", map[string]any{
			"token": run.Token,
			"skill": "edict-next-distribution",
			"title": "Assign the paired evidence to the existing cluster",
		},
	)
	managedCallDenied(
		t, ctx, client, "edict_delegate", map[string]any{
			"token": run.Token, "taskId": distributionTask.ID,
			"operations": []string{"cluster.signal.write"}, "scope": []string{"clusters/another-cluster"},
		},
	)
	managedCallDenied(
		t, ctx, client, "edict_delegate", map[string]any{
			"token": run.Token, "taskId": distributionTask.ID,
			"operations": []string{"cluster.write"}, "scope": []string{"clusters/" + historyFixtureCluster},
		},
	)
	distribution := managedCall[managed.Delegation](
		t, ctx, client, "edict_delegate", map[string]any{
			"token": run.Token, "taskId": distributionTask.ID,
			"operations": []string{"inbox.delete", "cluster.signal.write"},
			"scope":      []string{"inbox", "clusters/" + historyFixtureCluster},
		},
	)
	managedCall[managed.Plan](
		t,
		ctx,
		client,
		"edict_task_start",
		map[string]any{"token": distribution.Token, "agentId": "scripted-distribution-agent"},
	)
	for _, signal := range signals {
		read := managedCall[managed.File](t, ctx, client, "edict_read", map[string]any{"token": distribution.Token, "path": signal.Path})
		clusterPath := "clusters/" + historyFixtureCluster + "/signals/" + filepath.Base(signal.Path)
		copied := managedCall[managed.File](
			t, ctx, client, "edict_state_write", map[string]any{
				"token": distribution.Token, "path": clusterPath, "content": read.Content, "expectedHash": "",
			},
		)
		if copied.Content != signal.Content || copied.Hash != signal.Hash {
			t.Fatalf("distribution changed the signal evidence: %+v", copied)
		}
		managedCallDenied(
			t, ctx, client, "edict_state_delete", map[string]any{
				"token": distribution.Token, "path": signal.Path, "expectedHash": "stale-hash",
			},
		)
		managedCall[map[string]bool](
			t, ctx, client, "edict_state_delete", map[string]any{
				"token": distribution.Token, "path": signal.Path, "expectedHash": signal.Hash,
			},
		)
	}
	finished = managedCall[managed.Plan](
		t, ctx, client, "edict_task_finish", map[string]any{
			"token":  distribution.Token,
			"status": "completed",
			"result": "Distributed both signals without changing their provenance",
		},
	)
	assertManagedPlanOnDisk(
		t,
		stateDirectory,
		finished,
		distributionTask.ID,
		"completed",
		managerToken,
		run.Token,
		distribution.Token,
	)
	finished = managedCall[managed.Plan](
		t, ctx, client, "edict_task_finish", map[string]any{
			"token": run.Token, "status": "completed", "result": "Inbox drained into the existing cluster",
		},
	)
	assertManagedPlanOnDisk(
		t,
		stateDirectory,
		finished,
		runTask.ID,
		"completed",
		managerToken,
		run.Token,
		distribution.Token,
	)
	if !bytes.Equal(mustReadFile(t, sourcePath), sourceBefore) || !bytes.Equal(
		mustReadFile(t, clusterDescription),
		descriptionBefore,
	) {
		t.Fatal("the workflow changed source code or the supplied cluster description")
	}
	assertManagedCheckoutUnchanged(t, project)
	paths := managedCall[struct {
		Paths []string `json:"paths"`
	}](t, ctx, client, "edict_list", map[string]any{"token": managerToken, "prefix": "inbox"})
	if len(paths.Paths) != 0 {
		t.Fatalf("distribution left inbox entries: %v", paths.Paths)
	}
	t.Run(
		"HostSandboxProtectsPersistedState", func(t *testing.T) {
			binary, err := ResolveCodexExecutable()
			if err != nil {
				t.Skipf("Codex is required for the host sandbox check: %v", err)
			}
			scratch := filepath.Join(checkout.TestRoot, "sandbox-scratch")
			if err := os.MkdirAll(scratch, 0o755); err != nil {
				t.Fatal(err)
			}
			home, err := PrepareCodexHome(
				CodexHomeConfig{
					Directory:          filepath.Join(checkout.TestRoot, "sandbox-home"),
					Executable:         binary,
					PermissionsProfile: "managed-edict-test",
					ReadOnlyPaths:      []string{checkout.RepositoryDirectory, stateDirectory},
					WritableRoots:      []string{scratch},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			assertManagedSandboxDeniesDirectWrites(
				t,
				binary,
				home,
				checkout.RepositoryDirectory,
				stateDirectory,
				scratch,
			)
		},
	)
}

// This test launches a real Codex manager and native subagents. No IntelliJ
// server is needed for the extraction-only request.
func TestManagedEdictManagerSkill(t *testing.T) {
	//if testing.Short() || os.Getenv("EDICT_MANAGED_CODEX_TEST") != "1" {
	//	t.Skip("set EDICT_MANAGED_CODEX_TEST=1 to run the real Codex manager/subagent integration")
	//}
	requireCodexProvider(t)
	project := prepareManagedTestProject(t, historyFixtureFixRevision, historyFixtureProject)
	codex := prepareManagedCodex(t, project)

	result := codex.run(t, "Extract signals from the latest commit.", 15*time.Minute)

	codex.assertCompletedTasks(t, result, "edict-next-batch-signal-analysis", "edict-next-signal-analysis")
	assertManagedCommitSignals(t, project, historyFixSignalExpectation())
	assertManagedCheckoutUnchanged(t, project)
}

func assertManagedSandboxDeniesDirectWrites(t *testing.T, binary, home, repository, state, scratch string) {
	t.Helper()
	allowedProbe := filepath.Join(scratch, "sandbox-write-probe")
	deniedProbe := filepath.Join(state, "unmanaged-write-probe")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(
		ctx,
		binary,
		"sandbox",
		"-P",
		"managed-edict-test",
		"-C",
		repository,
		"--",
		"/bin/sh",
		"-c",
		`printf allowed > "$1" && printf forbidden > "$2"`,
		"edict-sandbox-probe",
		allowedProbe,
		deniedProbe,
	)
	command.Env = environmentWithOverrides(map[string]string{"CODEX_HOME": home})
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("the Codex sandbox allowed a direct persisted-state write")
	}
	if string(mustReadFile(t, allowedProbe)) != "allowed" {
		t.Fatalf("sandbox did not permit the configured scratch root: %s", output)
	}
	if _, statErr := os.Stat(deniedProbe); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("sandbox failed to isolate persisted state: %v\n%s", statErr, output)
	}
}

func managedSpawnedAgents(t *testing.T, home, output string) map[string]bool {
	t.Helper()
	spawned := make(map[string]bool)
	for _, line := range nonEmptyLines(output) {
		var event struct {
			Type string `json:"type"`
			Item struct {
				Type              string   `json:"type"`
				Tool              string   `json:"tool"`
				ReceiverThreadIDs []string `json:"receiver_thread_ids"`
			} `json:"item"`
		}
		if json.Unmarshal([]byte(line), &event) != nil || event.Type != "item.completed" ||
			!strings.Contains(event.Item.Type, "collab") || event.Item.Tool != "spawn_agent" {
			continue
		}
		for _, id := range event.Item.ReceiverThreadIDs {
			spawned[id] = true
		}
	}
	// Exec's public stream may show only root-thread events. Session rollouts
	// preserve the nested workers' native function calls and matched responses.
	err := filepath.WalkDir(
		filepath.Join(home, "sessions"), func(path string, entry fs.DirEntry, walkErr error) error {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			calls := make(map[string]bool)
			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 4096), 16<<20)
			for scanner.Scan() {
				var event struct {
					Type    string `json:"type"`
					Payload struct {
						Type   string `json:"type"`
						Name   string `json:"name"`
						CallID string `json:"call_id"`
						Output string `json:"output"`
					} `json:"payload"`
				}
				if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Type != "response_item" {
					continue
				}
				payload := event.Payload
				if payload.Type == "function_call" && (payload.Name == "spawn_agent" || strings.HasSuffix(
					payload.Name,
					".spawn_agent",
				)) {
					calls[payload.CallID] = true
				}
				if payload.Type == "function_call_output" && calls[payload.CallID] {
					var response struct {
						AgentID  string `json:"agent_id"`
						TaskName string `json:"task_name"`
					}
					if json.Unmarshal([]byte(payload.Output), &response) == nil {
						for _, id := range []string{response.AgentID, response.TaskName} {
							if id != "" {
								spawned[id] = true
							}
						}
					}
				}
			}
			return scanner.Err()
		},
	)
	if err != nil {
		t.Fatalf("read native subagent execution traces: %v", err)
	}
	return spawned
}

// Discover capability values from tool arguments/results, including JSON nested
// in text content. Plan IDs and content hashes have the same length but remain
// useful diagnostics. Raw traces stay in the private per-test output directory.
func redactManagedTrace(trace string, context ...string) string {
	pattern := regexp.MustCompile(`"token"\s*:\s*"([0-9a-f]{64})"`)
	for _, source := range append([]string{trace}, context...) {
		for _, match := range pattern.FindAllStringSubmatch(strings.ReplaceAll(source, `\`, ""), -1) {
			trace = strings.ReplaceAll(trace, match[1], "[redacted]")
		}
	}
	return trace
}

func requireManagedDistilleryFixture(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("DISTILLERY_TEST_REPO")) != "" {
		return
	}
	// Keep this test offline by default, using the local checkout supplied for
	// these integrations. CI can provide any equivalent clone via the env var.
	local := "/Users/alexey.afanasiev/prj/examples/distillery-test"
	if info, err := os.Stat(filepath.Join(local, ".git")); err != nil || info == nil {
		t.Skip("set DISTILLERY_TEST_REPO to a local distillery-test clone")
	}
	t.Setenv("DISTILLERY_TEST_REPO", local)
}

func newManagedIntegrationServer(t *testing.T, store *managed.Store, testRoot string) *mcp.Server {
	t.Helper()
	logs, err := managed.OpenLogs(filepath.Join(testRoot, "log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logs.Close() })
	return managed.NewServer(store, logs.Activity, logs.System)
}

func connectManagedTestServer(t *testing.T, store *managed.Store, testRoot string) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newManagedIntegrationServer(t, store, testRoot).Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client, err := mcp.NewClient(
		&mcp.Implementation{Name: "distillery-workflow-test", Version: "1"},
		nil,
	).Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func managedCall[T any](
	t *testing.T,
	ctx context.Context,
	client *mcp.ClientSession,
	name string,
	arguments map[string]any,
) T {
	t.Helper()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s protocol failure: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s failed: %s", name, managedResultText(result))
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode %s response %s: %v", name, data, err)
	}
	return value
}

func managedResultText(result *mcp.CallToolResult) string {
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func managedCallDenied(
	t *testing.T,
	ctx context.Context,
	client *mcp.ClientSession,
	name string,
	arguments map[string]any,
) {
	t.Helper()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s request failed at the protocol layer instead of authorization: %v", name, err)
	}
	if !result.IsError {
		t.Fatalf("%s unexpectedly accepted an unauthorized mutation", name)
	}
}

func assertManagedPlanOnDisk(
	t *testing.T,
	stateDirectory string,
	expected managed.Plan,
	taskID, status string,
	tokens ...string,
) {
	t.Helper()
	data := mustReadFile(t, filepath.Join(stateDirectory, "plans", expected.ID+".json"))
	for _, token := range tokens {
		if token != "" && bytes.Contains(data, []byte(token)) {
			t.Fatal("execution plan leaked a capability token")
		}
	}
	var persisted managed.Plan
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Revision != expected.Revision {
		t.Fatalf(
			"plan persistence lagged behind MCP response: disk=%d response=%d",
			persisted.Revision,
			expected.Revision,
		)
	}
	index := slices.IndexFunc(persisted.Tasks, func(task managed.Task) bool { return task.ID == taskID })
	if index < 0 || persisted.Tasks[index].Status != status || persisted.Tasks[index].Result == "" {
		t.Fatalf("task completion/result not persisted: %+v", persisted.Tasks)
	}
}

type managedCommitSignal struct {
	ID                 string                    `json:"id"`
	IdempotencyKey     string                    `json:"idempotencyKey"`
	FileRevision       qodanaHistoryFileRevision `json:"fileRevision"`
	Source             managedCommitSource       `json:"source"`
	Label              string                    `json:"label"`
	Description        string                    `json:"description"`
	SyntheticExampleID *string                   `json:"syntheticExampleId"`
	Provenance         map[string]string         `json:"provenance"`
}

type managedCommitSource struct {
	Type                   string `json:"type"`
	CommitRevision         string `json:"commitRevision"`
	ParentRevision         string `json:"parentRevision"`
	Message                string `json:"message"`
	DiffPositiveToNegative string `json:"diffPositiveToNegative"`
}

func managedFixtureSignals(t *testing.T, checkout distilleryTestCheckout) []managed.File {
	t.Helper()
	message := strings.TrimSpace(
		runCommand(
			t,
			checkout.RepositoryDirectory,
			checkout.GitBinary,
			"show",
			"-s",
			"--format=%B",
			historyFixtureFixRevision,
		),
	)
	diff := runCommand(
		t,
		checkout.RepositoryDirectory,
		checkout.GitBinary,
		"--no-pager",
		"diff",
		"--no-color",
		"--no-ext-diff",
		"--unified=200",
		historyFixtureSnapshot,
		historyFixtureFixRevision,
		"--",
		historyFixtureSourcePath,
		historyFixtureSourcePath,
	)
	var files []managed.File
	for i, evidence := range []struct {
		label, revision string
		line            int
	}{{"POSITIVE", historyFixtureSnapshot, 5}, {"NEGATIVE", historyFixtureFixRevision, 10}} {
		workItemID := "commit:" + historyFixtureFixRevision + ":" + historyFixtureSourcePath
		key := fmt.Sprintf(
			"%s\n%s\n%d\n%s\n%s\n%s\n%d:%d",
			distilleryTestRepositoryURL,
			workItemID,
			i,
			evidence.label,
			historyFixtureSourcePath,
			evidence.revision,
			evidence.line,
			evidence.line,
		)
		digest := sha256.Sum256([]byte(key))
		signal := managedCommitSignal{
			ID:             "s-" + hex.EncodeToString(digest[:])[:10],
			IdempotencyKey: key,
			FileRevision: qodanaHistoryFileRevision{
				Path:           historyFixtureSourcePath,
				Revision:       evidence.revision,
				ExpectedRanges: []historicalLineRange{{Start: evidence.line, End: evidence.line}},
			},
			Source: managedCommitSource{
				Type:                   "FromCommit",
				CommitRevision:         historyFixtureFixRevision,
				ParentRevision:         historyFixtureSnapshot,
				Message:                message,
				DiffPositiveToNegative: diff,
			},
			Label:       evidence.label,
			Description: "Use an explicit completion signal instead of sleeping to coordinate a worker",
			Provenance:  map[string]string{"workItemId": workItemID},
		}
		data, err := json.MarshalIndent(signal, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, managed.File{Path: signal.ID + ".json", Content: string(data)})
	}
	return files
}

func historyFixSignalExpectation() managedCommitExpectation {
	return managedCommitExpectation{
		Parent: historyFixtureSnapshot,
		Commit: historyFixtureFixRevision,
		Path:   historyFixtureSourcePath,
		Evidence: []managedSignalEvidence{
			{Label: "POSITIVE", Revision: historyFixtureSnapshot, Line: 5},
			{Label: "NEGATIVE", Revision: historyFixtureFixRevision, Line: 10},
		},
	}
}
