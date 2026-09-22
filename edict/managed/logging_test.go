// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestServerLogsRequestsAndResponsesWithoutCapabilities(t *testing.T) {
	directory := t.TempDir()
	logs, err := OpenLogs(directory)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, logs.Close()) })
	session, _, ctx := connectTestServer(t, logs)
	call := func(name string, args map[string]any) *mcp.CallToolResult {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, err)
		return result
	}
	args := map[string]any{"request": "Logged request", "steps": []Step{{Skill: "edict-next-run", Title: "Logged stage"}}}
	created := protocolOutput[PlanCreation](t, call("edict_plan_create", args))
	grant := protocolOutput[Delegation](t, call("edict_delegate", map[string]any{
		"token": created.Token, "taskId": created.Plan.Tasks[0].ID, "prompt": testPrompt(created.Plan.Tasks[0].Skill),
	}))
	require.Len(t, created.Token, 64)
	require.Len(t, grant.Token, 64)
	require.True(t, call("edict_plan_create", args).IsError)
	protocolOutput[TaskAssignment](t, call("edict_task_get", map[string]any{"token": grant.Token}))
	protocolOutput[Plan](t, call("edict_task_start", map[string]any{"token": grant.Token, "agentId": "logged-worker", "skill": grant.Skill}))
	cancelled := protocolOutput[Task](t, call("edict_task_add", map[string]any{
		"token": created.Token, "skill": "edict-next-batch-signal-analysis", "title": "Cancelled analysis",
	}))
	protocolOutput[Plan](t, call("edict_task_cancel", map[string]any{
		"token": created.Token, "taskId": cancelled.ID, "result": `{"details":{"cause":"Worker unavailable","debug":"internal cancellation details"}}`,
	}))
	digest := strings.Repeat("a", 64)
	resultJSON := fmt.Sprintf(`{"summary":"Inbox processed\nNo remaining work; hash %s; worker %s; manager %s","taskId":%q,"token":%q,"debug":"internal completion details"}`,
		digest, grant.Token, created.Token, grant.TaskID, grant.Token)
	protocolOutput[Plan](t, call("edict_task_finish", map[string]any{
		"token": grant.Token, "status": "completed", "result": resultJSON,
	}))
	require.True(t, call("edict_state_write", map[string]any{
		"token": grant.Token, "path": "inbox/s-one.json", "content": "{}", "expectedHash": "",
	}).IsError)
	protocolOutput[File](t, call("edict_read", map[string]any{"token": created.Token, "path": "plans/" + created.Plan.ID + ".json"}))

	// Concurrent clients must not interleave JSON records or correlation IDs.
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "edict_plan_get", Arguments: map[string]any{"token": created.Token}})
			if err != nil {
				t.Errorf("concurrent plan read: %v", err)
			}
		}()
	}
	wg.Wait()
	require.NoError(t, session.Close())
	data, err := os.ReadFile(filepath.Join(directory, "edict", "edict-mcp-system.log"))
	require.NoError(t, err)
	require.NotContains(t, string(data), created.Token)
	require.NotContains(t, string(data), grant.Token)
	require.Contains(t, string(data), "[redacted]")
	require.Contains(t, string(data), "Logged request")
	require.Contains(t, string(data), created.Plan.ID)
	require.Contains(t, string(data), "already succeeded")
	require.Contains(t, string(data), "internal cancellation details")
	require.Contains(t, string(data), "internal completion details")
	require.Contains(t, string(data), `"taskId":"`+grant.TaskID+`"`)
	require.Contains(t, string(data), digest)
	require.NotContains(t, string(data), `"taskId":"[redacted]"`)
	requests, responses := map[uint64]bool{}, map[uint64]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var entry struct {
			Time      string          `json:"time"`
			Message   string          `json:"msg"`
			Method    string          `json:"method"`
			RequestID uint64          `json:"requestId"`
			Params    json.RawMessage `json:"params"`
			Result    json.RawMessage `json:"result"`
			Error     string          `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		require.NotEmpty(t, entry.Time)
		switch entry.Message {
		case "request":
			require.NotEmpty(t, entry.Method)
			require.False(t, requests[entry.RequestID])
			require.NotEmpty(t, entry.Params)
			requests[entry.RequestID] = true
		case "response":
			require.True(t, requests[entry.RequestID])
			require.False(t, responses[entry.RequestID])
			require.True(t, len(entry.Result) != 0 || entry.Error != "")
			responses[entry.RequestID] = true
		}
	}
	require.Len(t, requests, 23) // initialize, eight mutations, fourteen reads
	require.Equal(t, requests, responses)

	activity, err := os.ReadFile(filepath.Join(directory, "edict", "edict-mcp.log"))
	require.NoError(t, err)
	unwrapped := strings.ReplaceAll(string(activity), "\n    ", "")
	expectedPrompt := strings.ReplaceAll(testPrompt(grant.Skill), "\n", "")
	require.Contains(t, unwrapped, expectedPrompt, "the full exact spawn prompt must be logged, except its token")
	agentOutput, err := os.ReadFile(logs.Agents.Name())
	require.NoError(t, err)
	require.Contains(t, strings.ReplaceAll(string(agentOutput), "\n    ", ""), expectedPrompt)
	require.NotContains(t, string(agentOutput), grant.Token)
	shortOutput, err := os.ReadFile(logs.AgentsShort.Name())
	require.NoError(t, err)
	short := strings.ReplaceAll(string(shortOutput), "\n    ", "")
	for _, want := range []string{
		`[edict-run/` + grant.TaskID[:8] + `] mcp: Task delegated by edict_manager/-`,
		`Read assigned task and prompt`,
		`Started task; assignment fetched and skill verified`,
		`Task "Logged stage" completed: Inbox processed`,
		`Write "inbox/s-one.json" failed: invalid or revoked capability`,
		`Read plan: 0 pending, 0 delegated, 0 running, 1 completed, 1 failed`,
		`Create plan "Logged request" failed:`,
		`Cancelled task "Cancelled analysis"`,
	} {
		require.Contains(t, short, want, "short log must retain MCP outcomes and failures")
	}
	for _, unwanted := range []string{expectedPrompt, "response:", "internal completion details", "internal cancellation details", created.Token, grant.Token} {
		require.NotContains(t, short, unwanted, "short log must omit payloads and capabilities")
	}
	for _, line := range strings.Split(string(shortOutput), "\n") {
		require.LessOrEqual(t, utf8.RuneCountInString(line), 120)
	}
	for _, want := range []string{
		`[edict_manager task=-] Plan ready: "Logged request"; manager assigned`,
		`[edict-run task=` + grant.TaskID[:8] + `] Task prompt assigned by edict_manager/-:`,
		`Create plan "Logged request" failed:`,
		`[edict-run task=` + grant.TaskID[:8] + `] Started task; assignment fetched and skill verified`,
		`[edict_manager task=-] Added task "Cancelled analysis" (edict-batch-signal-analysis task=` + cancelled.ID[:8] + `)`,
		`[edict_manager task=-] Cancelled task "Cancelled analysis" (edict-batch-signal-analysis task=` + cancelled.ID[:8] + `)`,
		`Structured result recorded`,
		`[edict-run task=` + grant.TaskID[:8] + `] Task "Logged stage" completed: Inbox processed No remaining work; hash ` + digest + `; worker [redacted]; manager [redacted]`,
		`[edict-run task=` + grant.TaskID[:8] + `] Write "inbox/s-one.json" failed: invalid or revoked capability`,
		`[edict_manager task=-] Read "plans/` + created.Plan.ID + `.json"`,
		`[edict_manager task=-] Read plan: 0 pending, 0 delegated, 0 running, 1 completed, 1 failed`,
	} {
		require.Contains(t, unwrapped, want)
	}
	for _, want := range []string{grant.TaskID, "internal cancellation details", "internal completion details", "token: '[redacted]'",
		"edict_plan_get response:", "edict_task_finish response:", "edict_state_write response:", "error: invalid or revoked capability"} {
		require.Contains(t, unwrapped, want)
	}
	for _, unwanted := range []string{created.Token, grant.Token,
		"requestId", "sessionId", "structuredContent", "durationMs", `"method"`, `"params"`} {
		require.NotContains(t, string(activity), unwanted)
	}
	lines := strings.Split(strings.TrimSpace(string(activity)), "\n")
	headers := 0
	for _, line := range lines {
		require.LessOrEqual(t, utf8.RuneCountInString(line), 120)
		if strings.HasPrefix(line, "    ") {
			continue
		}
		headers++
		require.Regexp(t, `^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} \[[a-z_-]+ task=(?:[0-9a-f]{8}|-)\] .+`, line)
	}
	require.Equal(t, 44, headers, "each handler logs its summary and complete response")
}

func TestActivityLogsCallerAcrossDelegationAndSharedSessionReads(t *testing.T) {
	directory := t.TempDir()
	logs, err := OpenLogs(directory)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, logs.Close()) })
	session, _, ctx := connectTestServer(t, logs)
	call := func(name string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, err)
		require.False(t, result.IsError, "%s: %v", name, result.Content)
		return result
	}
	call("edict_plan_get", nil) // Bootstrap has no caller token yet.
	created := protocolOutput[PlanCreation](t, call("edict_plan_create", map[string]any{
		"request": "Extract signals", "steps": []Step{{Skill: "edict-next-batch-signal-analysis", Title: "Extract signals"}},
	}))
	batch := protocolOutput[Delegation](t, call("edict_delegate", map[string]any{
		"token": created.Token, "taskId": created.Plan.Tasks[0].ID, "prompt": testPrompt(created.Plan.Tasks[0].Skill),
		"operations": []string{"inbox.write"}, "scope": []string{"inbox"},
	}))
	call("edict_task_get", map[string]any{"token": batch.Token})
	call("edict_task_start", map[string]any{"token": batch.Token, "agentId": "batch-worker", "skill": batch.Skill})
	child := protocolOutput[Task](t, call("edict_task_add", map[string]any{
		"token": batch.Token, "skill": "edict-next-signal-analysis", "title": "Review commit",
	}))
	analysis := protocolOutput[Delegation](t, call("edict_delegate", map[string]any{"token": batch.Token, "taskId": child.ID, "prompt": testPrompt(child.Skill)}))
	call("edict_task_get", map[string]any{"token": analysis.Token})
	call("edict_task_start", map[string]any{"token": analysis.Token, "agentId": "analysis-worker", "skill": analysis.Skill})
	call("edict_registry", map[string]any{"token": analysis.Token})
	call("edict_list", map[string]any{"token": batch.Token, "prefix": "inbox"})
	call("edict_plan_get", map[string]any{"token": created.Token})
	batchFile, analysisFile := validTestSignal(t, "batch"), validTestSignal(t, "analysis")
	for _, file := range []File{batchFile, analysisFile} {
		call("edict_state_write", map[string]any{"token": batch.Token, "path": file.Path, "content": file.Content, "expectedHash": ""})
	}
	batchPrefix := "[edict-batch-signal-analysis task=" + batch.TaskID[:8] + "] "
	analysisPrefix := "[edict-signal-analysis task=" + child.ID[:8] + "] "
	// Both workers share one MCP session; attribution must follow each request's
	// token, not whichever worker most recently called the server.
	var wg sync.WaitGroup
	for _, caller := range []struct{ token, path string }{
		{batch.Token, batchFile.Path}, {analysis.Token, analysisFile.Path},
	} {
		for range 6 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := session.CallTool(ctx, &mcp.CallToolParams{
					Name: "edict_read", Arguments: map[string]any{"token": caller.token, "path": caller.path},
				})
				if err != nil || result.IsError {
					t.Errorf("concurrent attributed read: %v, %v", err, result)
				}
			}()
		}
	}
	wg.Wait()
	denied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "edict_task_add", Arguments: map[string]any{
		"token": analysis.Token, "skill": "edict-next-run", "title": "Unpermitted child",
	}})
	require.NoError(t, err)
	require.True(t, denied.IsError)
	call("edict_task_cancel", map[string]any{"token": batch.Token, "taskId": child.ID, "result": "Review cancelled"})
	call("edict_list", map[string]any{"prefix": "inbox"}) // Still public, never guessed from prior calls.
	call("edict_registry", map[string]any{"token": "unrecognized-token"})
	require.NoError(t, session.Close())

	activity, err := os.ReadFile(filepath.Join(directory, "edict", "edict-mcp.log"))
	require.NoError(t, err)
	text := strings.ReplaceAll(string(activity), "\n    ", "")
	for _, want := range []string{
		`[anonymous task=-] Read plan: no plan created yet`,
		batchPrefix + `Added task "Review commit" (edict-signal-analysis task=` + child.ID[:8] + `)`,
		analysisPrefix + `Task prompt assigned by edict-batch-signal-analysis/` + batch.TaskID[:8] + `:`,
		batchPrefix + `Cancelled task "Review commit" (edict-signal-analysis task=` + child.ID[:8] + `): Review cancelled`,
		analysisPrefix + `Add task "Unpermitted child" (edict-run) failed:`,
		analysisPrefix + `Read skill registry:`,
		batchPrefix + `Listed files under "inbox":`,
		`[edict_manager task=-] Read plan:`,
		`[anonymous task=-] Listed files under "inbox":`,
		`[unknown task=-] Read skill registry:`,
	} {
		require.Contains(t, text, want)
	}
	require.Equal(t, 6, strings.Count(text, batchPrefix+fmt.Sprintf("Read %q", batchFile.Path)))
	require.Equal(t, 6, strings.Count(text, analysisPrefix+fmt.Sprintf("Read %q", analysisFile.Path)))
	for _, token := range []string{created.Token, batch.Token, analysis.Token} {
		require.NotContains(t, text, token)
	}
	require.NotContains(t, text, "[edict-mcp task=-]")
}

func TestOpenLogsAppendAndReportInvalidDirectory(t *testing.T) {
	directory := t.TempDir()
	for _, line := range []string{"first\n", "second\n"} {
		logs, err := OpenLogs(directory)
		require.NoError(t, err)
		for _, file := range []*os.File{logs.Activity, logs.System, logs.Agents, logs.AgentsShort} {
			_, err = file.WriteString(line)
			require.NoError(t, err)
		}
		require.NoError(t, logs.Close())
	}
	for _, name := range []string{"edict-mcp.log", "edict-mcp-system.log", "edict-agents.log", "edict-agent-short.log"} {
		data, err := os.ReadFile(filepath.Join(directory, "edict", name))
		require.NoError(t, err)
		require.Equal(t, "first\nsecond\n", string(data))
	}
	_, err := OpenLogs(" ")
	require.Error(t, err)
	_, err = OpenLogs(filepath.Join(directory, "edict", "edict-mcp-system.log"))
	require.ErrorContains(t, err, "creating Edict log directory")
}
