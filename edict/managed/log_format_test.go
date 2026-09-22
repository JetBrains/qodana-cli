// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestActivityPreservesActualSkillFilePaths(t *testing.T) {
	var output bytes.Buffer
	logger := newActivityLogger(&output)
	path := "managed-edict-next-signal-analysis/SKILL.md"
	logger.printf(Task{Skill: "edict-next-signal-analysis"}, "Read the assigned skill at %s", path)
	require.Contains(t, output.String(), "[edict-signal-analysis task=-]")
	require.Contains(t, output.String(), path, "display shortening must not corrupt actionable file paths")
}

func TestAgentRecordWrapsAt120CharactersWithoutLosingText(t *testing.T) {
	task := Task{Skill: "edict-next-signal-analysis", ID: strings.Repeat("a", 64)}
	for _, text := range []string{
		strings.Repeat("A readable sentence. ", 20),
		strings.Repeat("Café 世界 👋 ", 40),
		"https://example.test/" + strings.Repeat("a", 350),
		"{\n  \"description\": \"" + strings.Repeat("source evidence ", 40) + "\"\n}",
	} {
		record := formatAgentRecord(time.Now(), task, "final", text)
		for _, line := range strings.Split(strings.TrimSuffix(record, "\n"), "\n") {
			require.LessOrEqual(t, utf8.RuneCountInString(line), 120, "%s", line)
			require.True(t, utf8.ValidString(line))
		}
		require.Contains(t, record, "[edict-signal-analysis/aaaaaaaa] final: ")
		unwrapped := strings.ReplaceAll(strings.TrimSuffix(record, "\n"), "\n    ", "")
		body := strings.SplitN(unwrapped, "] final: ", 2)[1]
		require.Equal(t, strings.ReplaceAll(text, "\n", ""), body)
		require.NotContains(t, record, "agent=")
	}
}

func TestCombinedAgentLogIncludesAttributedMCPActivity(t *testing.T) {
	logs, err := OpenLogs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, logs.Close()) })
	store, manager, _ := testStore(t)
	agents := NewAgentLogger(store, logs.Agents, logs.AgentsShort)
	server := NewServer(store, logs.Activity, logs.System, agents)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	client, err := mcp.NewClient(&mcp.Implementation{Name: "combined-log-test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	batch := worker(t, store, manager, "edict-next-batch-signal-analysis", nil, nil)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			agents.Record(AgentMessage{Time: time.Now(), AgentID: "agent-" + batch.TaskID, Phase: "commentary", Text: "Reading the plan " + batch.Token})
			if err := agents.Flush(false); err != nil {
				t.Error(err)
			}
			result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "edict_plan_get", Arguments: map[string]any{"token": batch.Token}})
			if err != nil || result.IsError {
				t.Errorf("read plan: %v %v", err, result)
			}
		}()
	}
	wg.Wait()
	data, err := os.ReadFile(logs.Agents.Name())
	require.NoError(t, err)
	output := string(data)
	prefix := "[edict-batch-signal-analysis/" + batch.TaskID[:8] + "] "
	require.Equal(t, 8, strings.Count(output, prefix+"commentary: Reading the plan [redacted]"))
	require.Equal(t, 8, strings.Count(output, prefix+"mcp: Read plan:"))
	require.Equal(t, 8, strings.Count(output, prefix+"mcp: edict_plan_get response:"))
	require.Contains(t, output, "id: "+batch.TaskID)
	require.NotContains(t, output, batch.Token)
	data, err = os.ReadFile(logs.AgentsShort.Name())
	require.NoError(t, err)
	short := string(data)
	require.Equal(t, 8, strings.Count(short, prefix+"commentary: Reading the plan [redacted]"))
	require.Equal(t, 8, strings.Count(short, prefix+"mcp: Read plan:"))
	require.NotContains(t, short, "edict_plan_get response:")
	require.NotContains(t, short, "id: "+batch.TaskID)
	require.NotContains(t, short, batch.Token)
	for _, text := range []string{output, short} {
		for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
			require.LessOrEqual(t, utf8.RuneCountInString(line), 120)
		}
	}
}

func TestReadableResponsesPreserveFullReportsAndRedactOnlyTokens(t *testing.T) {
	store, manager, _ := testStore(t)
	var activity, agents bytes.Buffer
	logger := newActivityLogger(&activity)
	logger.agents = NewAgentLogger(store, &agents, nil)
	caller := Task{Skill: "edict_manager"}
	logger.remember(manager, caller)
	// Exercise JSON within JSON, multiline source, large exact numbers, and a
	// report larger than the short status summary's limit.
	report := `{"workItemId":"commit-19475f69ff6ed87a","source":{"commitRevision":"19475f69ff6ed87a68712b4ad9d55938f3868b6e","diff":"-old\n+new\n"},"count":9007199254740993,"token":"not-an-issued-token","description":"` + strings.Repeat("evidence ", 100) + manager + `"}`
	response := File{Path: "inbox/s-example.json", Hash: strings.Repeat("f", 64), Content: report}
	logger.response(caller, "edict_read", response, nil)
	require.Equal(t, report, response.Content, "logging must not change the actual response")
	for _, output := range []string{activity.String(), agents.String()} {
		unwrapped := strings.ReplaceAll(output, "\n    ", "")
		for _, want := range []string{
			"edict_read response:", "workItemId: commit-19475f69ff6ed87a", "commitRevision: 19475f69ff6ed87a68712b4ad9d55938f3868b6e",
			"count: 9007199254740993", "hash: " + response.Hash, "token: '[redacted]'", strings.Repeat("evidence ", 100) + "[redacted]",
		} {
			require.Contains(t, unwrapped, want)
		}
		require.Contains(t, output, "diff: |\n")
		require.Contains(t, output, "-old\n")
		require.Contains(t, output, "+new\n")
		require.NotContains(t, output, manager)
		require.NotContains(t, output, "not-an-issued-token")
		for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
			require.LessOrEqual(t, utf8.RuneCountInString(line), 120)
		}
	}
}
