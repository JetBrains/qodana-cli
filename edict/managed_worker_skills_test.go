// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package edict

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/edict/managed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verify actual skill reads from runtime calls and their returned file content,
// rather than accepting a worker's prose claim or its task_start declaration.
func assertManagedWorkerSkills(t *testing.T, home string, plan *managed.Plan) {
	t.Helper()
	loaded, err := managedWorkerSkillReads(home)
	if err != nil {
		t.Fatalf("inspect managed worker skill loads: %v", err)
	}
	for _, task := range plan.Tasks {
		assert.True(t, loaded[task.AgentID]["managed-"+task.Skill], "task %s must read its assigned managed-%s/SKILL.md", task.ID, task.Skill)
		assert.False(t, loaded[task.AgentID]["edict_manager"], "task %s must load its worker skill, not the root manager skill", task.ID)
	}
}

func managedWorkerSkillReads(home string) (map[string]map[string]bool, error) {
	loaded := make(map[string]map[string]bool)
	skillPath := regexp.MustCompile(`(edict_manager|managed-edict-[a-z-]+)/SKILL\.md`)
	err := filepath.WalkDir(filepath.Join(home, "sessions"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		var identity codexAgentSession
		reads, calls := make(map[string]bool), make(map[string][]string)
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 4096), 16<<20)
		for scanner.Scan() {
			var event struct {
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				return err
			}
			if event.Type == "session_meta" {
				if err := identity.consume(scanner.Bytes()); err != nil {
					return err
				}
			}
			if event.Type != "response_item" {
				continue
			}
			var payload struct {
				Type      string          `json:"type"`
				CallID    string          `json:"call_id"`
				Input     string          `json:"input"`
				Arguments string          `json:"arguments"`
				Output    json.RawMessage `json:"output"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return err
			}
			switch payload.Type {
			case "function_call", "custom_tool_call":
				for _, match := range skillPath.FindAllStringSubmatch(payload.Input+payload.Arguments, -1) {
					calls[payload.CallID] = append(calls[payload.CallID], match[1])
				}
			case "function_call_output", "custom_tool_call_output":
				for _, skill := range calls[payload.CallID] {
					// Codex emits either a string or an array of content blocks.
					// The ASCII skill header is unchanged in either JSON encoding.
					if strings.Contains(string(payload.Output), "name: "+skill) {
						reads[skill] = true
					}
				}
			}
		}
		loaded[identity.id] = reads
		if identity.path != "" {
			loaded[identity.path] = reads
		}
		return scanner.Err()
	})
	return loaded, err
}

func TestManagedWorkerSkillReadsSupportRuntimeOutputFormats(t *testing.T) {
	skill := "managed-edict-next-signal-analysis"
	header := "---\nname: " + skill + "\n---\n"
	for _, tc := range []struct {
		name   string
		output any
		read   bool
	}{
		{"string", header, true},
		{"blocks", []map[string]string{{"type": "input_text", "text": header}}, true},
		{"failed read", "cat: no such file", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(home, "sessions"), 0o700))
			file, err := os.Create(filepath.Join(home, "sessions", "worker.jsonl"))
			require.NoError(t, err)
			encoder := json.NewEncoder(file)
			require.NoError(t, encoder.Encode(map[string]any{"type": "session_meta", "payload": map[string]any{
				"id": "worker", "source": map[string]any{"subagent": map[string]any{"thread_spawn": map[string]string{"agent_path": "/root/worker"}}},
			}}))
			for _, payload := range []map[string]any{
				{"type": "custom_tool_call", "call_id": "read", "input": "cat /skills/" + skill + "/SKILL.md"},
				{"type": "custom_tool_call_output", "call_id": "read", "output": tc.output},
				// A skill name in an unrelated output must not count as a read.
				{"type": "function_call_output", "call_id": "unrelated", "output": "name: edict_manager"},
			} {
				require.NoError(t, encoder.Encode(map[string]any{"type": "response_item", "payload": payload}))
			}
			require.NoError(t, file.Close())
			loaded, err := managedWorkerSkillReads(home)
			require.NoError(t, err)
			require.Equal(t, tc.read, loaded["worker"][skill])
			require.Equal(t, loaded["worker"], loaded["/root/worker"])
			require.False(t, loaded["worker"]["edict_manager"])
		})
	}
}
