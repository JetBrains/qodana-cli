/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package managed

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectTestServer(t *testing.T, logs *Logs) (*mcp.ClientSession, *Store, context.Context) {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	var server *mcp.Server
	if logs == nil {
		server = NewServer(store, nil, nil)
	} else {
		server = NewServer(store, logs.Activity, logs.System)
	}
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "edict-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, store, ctx
}

func TestServerDiscoversTypedTools(t *testing.T) {
	session, _, ctx := connectTestServer(t, nil)
	if got := session.InitializeResult().ServerInfo.Name; got != "edict-mcp" {
		t.Fatalf("unexpected server identity %q", got)
	}
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{
		"edict_registry": true, "edict_read": true, "edict_list": true, "edict_plan_get": true,
		"edict_plan_create": false, "edict_task_add": false, "edict_delegate": false,
		"edict_task_start": false, "edict_task_finish": false, "edict_task_cancel": false,
		"edict_state_write": false, "edict_state_delete": false,
	}
	if len(result.Tools) != len(expected) {
		t.Fatalf("got %d tools, expected %d", len(result.Tools), len(expected))
	}
	for _, tool := range result.Tools {
		readOnly, exists := expected[tool.Name]
		if !exists {
			t.Errorf("unexpected tool %s", tool.Name)
		}
		if tool.Description == "" || tool.InputSchema == nil {
			t.Errorf("tool %s lacks a description or input schema", tool.Name)
		}
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint != readOnly {
			t.Errorf("tool %s has incorrect read-only annotation", tool.Name)
		}
		if !readOnly {
			encoded, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			var schema struct {
				Required []string `json:"required"`
			}
			if err := json.Unmarshal(encoded, &schema); err != nil {
				t.Fatal(err)
			}
			if tool.Name == "edict_plan_create" {
				if strings.Contains(string(encoded), `"token"`) {
					t.Fatal("plan creation must not accept a token")
				}
			} else if !slices.Contains(schema.Required, "token") {
				t.Errorf("mutation tool %s does not require a token", tool.Name)
			}
		}
	}
}

func TestServerAllowsPublicDiscoveryButRequiresTokensForMutations(t *testing.T) {
	session, _, ctx := connectTestServer(t, nil)
	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "edict_plan_create", Arguments: map[string]any{
		"request": "Produce review rules", "steps": []Step{{Skill: "edict-next-run", Title: "Process inbox"}},
	}})
	if err != nil || created.IsError {
		t.Fatalf("create manager: %v, %+v", err, created)
	}
	managerToken := protocolOutput[PlanCreation](t, created).Token
	for _, name := range []string{"edict_registry", "edict_list", "edict_plan_get"} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
		if err != nil || result.IsError {
			t.Fatalf("public %s failed: %v, %+v", name, err, result)
		}
		encoded, _ := json.Marshal(result)
		if strings.Contains(string(encoded), managerToken) {
			t.Fatalf("public %s exposed manager capability", name)
		}
	}
	mutations := map[string]map[string]any{
		"edict_task_add":     {"skill": "edict-next-run", "title": "Read recent changes"},
		"edict_delegate":     {"taskId": "unknown"},
		"edict_task_start":   {"agentId": "subagent-1"},
		"edict_task_finish":  {"status": "completed", "result": "done"},
		"edict_task_cancel":  {"taskId": "unknown", "result": "Worker lost"},
		"edict_state_write":  {"path": "signals/signal.md", "content": "injected", "expectedHash": ""},
		"edict_state_delete": {"path": "signals/signal.md", "expectedHash": "unknown"},
	}
	for name, arguments := range mutations {
		t.Run(name, func(t *testing.T) {
			arguments["token"] = "unmanaged-invalid-token"
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
			if err != nil {
				t.Fatalf("authorization error must be an MCP tool result: %v", err)
			}
			if !result.IsError {
				t.Fatalf("unauthorized mutation succeeded: %+v", result)
			}
			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), "unmanaged-invalid-token") || strings.Contains(string(encoded), managerToken) {
				t.Fatal("authorization error exposes a capability")
			}
		})
	}
}

func TestServerPlanCreationAndInputErrors(t *testing.T) {
	session, _, ctx := connectTestServer(t, nil)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "edict_plan_create", Arguments: map[string]any{"request": "Find reusable rules", "steps": []Step{
			{Skill: "edict-next-batch-signal-analysis", Title: "Analyze recent changes"},
			{Skill: "edict-next-run", Title: "Generate inspections from signals"},
		}},
	})
	if err != nil || result.IsError {
		t.Fatalf("creating default plan: %v, %+v", err, result)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "edict-next-run") || !strings.Contains(string(encoded), "edict-next-batch-signal-analysis") {
		t.Fatalf("default plan missing expected pipeline: %s", encoded)
	}
	creation := protocolOutput[PlanCreation](t, result)
	token := creation.Token
	if len(token) != 64 || creation.Plan == nil {
		t.Fatal("plan creation did not return the plan and a manager capability")
	}
	duplicate, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "edict_plan_create", Arguments: map[string]any{"request": "Take over manager", "steps": []Step{
			{Skill: "edict-next-run", Title: "Replace the existing plan"},
		}},
	})
	if err != nil || !duplicate.IsError {
		t.Fatalf("second manager claim was accepted: %v, %+v", err, duplicate)
	}
	denial, err := json.Marshal(duplicate)
	if err != nil || strings.Contains(string(denial), token) {
		t.Fatal("duplicate claim exposed the manager token")
	}
	for _, call := range []*mcp.CallToolParams{
		{Name: "edict_state_write", Arguments: map[string]any{"path": "signals/x.md"}},
		{Name: "edict_delegate", Arguments: map[string]any{"token": token, "taskId": 42}},
		{Name: "unknown-tool", Arguments: map[string]any{}},
	} {
		result, err := session.CallTool(ctx, call)
		if err == nil && !result.IsError {
			t.Fatalf("invalid %s request succeeded: %+v", call.Name, result)
		}
	}
}

func TestServerDelegationScopesAndRevocation(t *testing.T) {
	session, _, ctx := connectTestServer(t, nil)
	signal := validTestSignal(t, "protocol")
	call := func(name string, args map[string]any, wantError bool) *mcp.CallToolResult {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s protocol error: %v", name, err)
		}
		if result.IsError != wantError {
			t.Fatalf("%s error=%v, expected %v: %v", name, result.GetError(), wantError, result.Content)
		}
		return result
	}
	creation := protocolOutput[PlanCreation](t, call("edict_plan_create", map[string]any{
		"request": "Analyze one change",
		"steps":   []Step{{Skill: "edict-next-batch-signal-analysis", Title: "Analyze signal"}},
	}, false))
	plan, token := creation.Plan, creation.Token
	grant := protocolOutput[Delegation](t, call("edict_delegate", map[string]any{
		"token": token, "taskId": plan.Tasks[0].ID, "operations": []string{"inbox.write"}, "scope": []string{signal.Path},
	}, false))
	write := map[string]any{"token": grant.Token, "path": signal.Path, "content": signal.Content, "expectedHash": ""}
	call("edict_state_write", write, true)
	call("edict_task_start", map[string]any{"token": grant.Token, "agentId": "batch-worker"}, false)
	file := protocolOutput[File](t, call("edict_state_write", write, false))
	read := protocolOutput[File](t, call("edict_read", map[string]any{"path": file.Path}, false))
	if read != file || file.Hash == "" {
		t.Fatalf("state write/read roundtrip failed: %+v, %+v", file, read)
	}
	write["content"] = strings.Replace(signal.Content, "Wait for completion", "Use synchronization", 1)
	call("edict_state_write", write, true)
	write["expectedHash"] = file.Hash
	updated := protocolOutput[File](t, call("edict_state_write", write, false))
	if updated.Hash == file.Hash {
		t.Fatal("replacement failed to change optimistic concurrency hash")
	}
	call("edict_state_write", map[string]any{
		"token": grant.Token, "path": "inbox/outside-scope.json", "content": `{}`, "expectedHash": "",
	}, true)
	call("edict_task_add", map[string]any{
		"token": grant.Token, "skill": "edict-next-run", "title": "Disallowed delegation",
	}, true)
	finished := protocolOutput[Plan](t, call("edict_task_finish", map[string]any{
		"token": grant.Token, "status": "completed", "result": "One signal extracted",
	}, false))
	if finished.Tasks[0].Status != "completed" || finished.Revision <= plan.Revision {
		t.Fatalf("task progress was not persisted: %+v", finished)
	}
	write["expectedHash"] = updated.Hash
	call("edict_state_write", write, true)
}

func protocolOutput[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output T
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	return output
}
