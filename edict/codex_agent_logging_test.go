// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package edict

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/JetBrains/qodana-cli/edict/managed"
	"github.com/stretchr/testify/require"
)

func TestCodexAgentCollectorTracksNestedOutputWithoutDuplicates(t *testing.T) {
	home := t.TempDir()
	store, err := managed.NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	created, err := store.CreatePlan("Analyze", []managed.Step{{Skill: "edict-next-batch-signal-analysis", Title: "Batch"}})
	require.NoError(t, err)
	batch, err := store.Delegate(created.Token, created.Plan.Tasks[0].ID, nil, nil, "$managed-edict-next-batch-signal-analysis\nRead /skills/managed-edict-next-batch-signal-analysis/SKILL.md. Analyze the fixture.")
	require.NoError(t, err)
	_, err = store.ReadTask(batch.Token)
	require.NoError(t, err)
	_, err = store.StartTask(batch.Token, "batch-thread", batch.Skill)
	require.NoError(t, err)
	task, err := store.AddTask(batch.Token, "edict-next-signal-analysis", "Review")
	require.NoError(t, err)
	child, err := store.Delegate(batch.Token, task.ID, nil, nil, "$managed-edict-next-signal-analysis\nRead /skills/managed-edict-next-signal-analysis/SKILL.md. Inspect the commit.")
	require.NoError(t, err)
	_, err = store.ReadTask(child.Token)
	require.NoError(t, err)
	_, err = store.StartTask(child.Token, "/root/batch/review", child.Skill)
	require.NoError(t, err)

	var log bytes.Buffer
	stdout := filepath.Join(home, "stdout.jsonl")
	require.NoError(t, os.WriteFile(stdout, []byte("{\"type\":\"thread.started\",\"thread_id\":\"root-thread\"}\n"), 0o600))
	collector := newCodexAgentCollector(home, stdout, managed.NewAgentLogger(store, &log))
	sessions := filepath.Join(home, "sessions")
	require.NoError(t, os.Mkdir(sessions, 0o700))
	writeEvent := func(file string, event any) {
		t.Helper()
		data, err := json.Marshal(event)
		require.NoError(t, err)
		f, err := os.OpenFile(filepath.Join(sessions, file), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		require.NoError(t, err)
		_, err = f.Write(append(data, '\n'))
		require.NoError(t, err)
		require.NoError(t, f.Close())
	}
	meta := func(file, id, parent, path string) {
		var source any = "exec"
		if parent != "" {
			source = map[string]any{"subagent": map[string]any{"thread_spawn": map[string]any{"parent_thread_id": parent, "agent_path": path}}}
		}
		writeEvent(file, map[string]any{"type": "session_meta", "payload": map[string]any{"id": id, "source": source}})
	}
	message := func(role, phase, text string) map[string]any {
		return map[string]any{"type": "response_item", "timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"payload": map[string]any{"type": "message", "role": role, "phase": phase, "content": []map[string]string{{"type": "output_text", "text": text}}}}
	}
	meta("z-root.jsonl", "root-thread", "", "")
	meta("b-batch.jsonl", "batch-thread", "root-thread", "/root/batch")
	meta("a-child.jsonl", "child-thread", "batch-thread", "/root/batch/review")
	meta("unrelated.jsonl", "other-root", "", "")
	writeEvent("z-root.jsonl", message("assistant", "commentary", "Manager output"))
	writeEvent("b-batch.jsonl", message("assistant", "commentary", "Batch output"))
	writeEvent("a-child.jsonl", message("assistant", "final_answer", "Child output token "+child.Token))
	writeEvent("unrelated.jsonl", message("assistant", "final_answer", "Unrelated session"))
	writeEvent("z-root.jsonl", message("user", "", "Private prompt"))
	writeEvent("z-root.jsonl", message("assistant", "analysis", "Private reasoning"))
	writeEvent("z-root.jsonl", map[string]any{"type": "response_item", "payload": map[string]any{"type": "agent_message", "content": "Duplicate received worker output"}})
	writeEvent("z-root.jsonl", map[string]any{"type": "response_item", "payload": map[string]any{"type": "function_call_output", "output": "Raw tool output"}})
	require.NoError(t, collector.scan(false))
	require.Contains(t, log.String(), "[edict_manager/-] commentary: Manager output")
	require.Contains(t, log.String(), "[edict-batch-signal-analysis/"+batch.TaskID[:8]+"] commentary: Batch output")
	require.Contains(t, log.String(), "[edict-signal-analysis/"+child.TaskID[:8]+"] final: Child output token [redacted]")
	for _, absent := range []string{"Unrelated session", "Private prompt", "Private reasoning", "Raw tool output", "Duplicate received worker output", child.Token} {
		require.NotContains(t, log.String(), absent)
	}

	// The tail must wait for the rest of an event and emit it exactly once.
	event, err := json.Marshal(message("assistant", "final_answer", "Final manager output"))
	require.NoError(t, err)
	f, err := os.OpenFile(filepath.Join(sessions, "z-root.jsonl"), os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.Write(event[:len(event)/2])
	require.NoError(t, err)
	require.NoError(t, collector.scan(false))
	require.NotContains(t, log.String(), "Final manager output")
	_, err = f.Write(event[len(event)/2:]) // Final flush also supports no trailing newline.
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, collector.scan(true))
	require.NoError(t, collector.scan(true))
	for _, text := range []string{"Manager output", "Batch output", "Child output", "Final manager output"} {
		require.Equal(t, 1, strings.Count(log.String(), text))
	}
}

func TestRunCodexCapturesAgentOutputOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX Codex stub")
	}
	temporary := t.TempDir()
	store, err := managed.NewStore(filepath.Join(temporary, "state"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	var log bytes.Buffer
	executable := filepath.Join(temporary, "codex-stub")
	stub := `#!/bin/sh
mkdir -p "$CODEX_HOME/sessions"
printf '%s\n' '{"type":"thread.started","thread_id":"failed-root"}'
printf '%s\n' '{"type":"session_meta","payload":{"id":"failed-root","source":"exec"}}' '{"timestamp":"2026-09-21T16:00:00Z","type":"response_item","payload":{"type":"message","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":"Partial output before failure"}]}}' > "$CODEX_HOME/sessions/root.jsonl"
exit 7
`
	require.NoError(t, os.WriteFile(executable, []byte(stub), 0o700))
	_, err = RunCodex(context.Background(), CodexRunConfig{
		Executable: executable, HomeDirectory: filepath.Join(temporary, "home"), WorkingDirectory: temporary,
		OutputDirectory: filepath.Join(temporary, "trace"), Model: "test", Prompt: "test",
		AgentLogger: managed.NewAgentLogger(store, &log),
	})
	require.ErrorContains(t, err, "exit status 7")
	require.Contains(t, log.String(), "[edict_manager/-] commentary: Partial output before failure")
}
