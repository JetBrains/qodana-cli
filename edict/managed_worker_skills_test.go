// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package edict

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"maps"
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
func assertManagedWorkerSetup(t *testing.T, home string, plan *managed.Plan) {
	t.Helper()
	loaded, err := managedWorkerSkillReads(home)
	if err != nil {
		t.Fatalf("inspect managed worker skill loads: %v", err)
	}
	for _, task := range plan.Tasks {
		worker := loaded[task.AgentID]
		assert.True(t, worker.SkillsAtStart["managed-"+task.Skill], "task %s must read its assigned managed-%s/SKILL.md before starting", task.ID, task.Skill)
		assert.False(t, worker.Skills["edict_manager"], "task %s must load its worker skill, not the root manager skill", task.ID)
		assert.True(t, worker.FetchedBeforeStart, "worker %s must fetch its assignment in its own runtime session before startup", task.ID)
		assert.Equal(t, managed.TaskAssignment{TaskID: task.ID, Skill: task.Skill, SkillPath: "managed-" + task.Skill + "/SKILL.md", Prompt: task.Prompt}, worker.Assignment,
			"worker %s must execute the exact assignment returned by MCP", task.ID)
	}
}

type managedWorkerSetup struct {
	Skills             map[string]bool
	SkillsAtStart      map[string]bool
	Assignment         managed.TaskAssignment
	FetchedBeforeStart bool
}

func managedWorkerSkillReads(home string) (map[string]managedWorkerSetup, error) {
	loaded := make(map[string]managedWorkerSetup)
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
		setup := managedWorkerSetup{Skills: reads}
		started := false
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
			if event.Type == "event_msg" {
				var payload struct {
					Type string `json:"type"`
					Item struct {
						Server   string          `json:"server"`
						Tool     string          `json:"tool"`
						Command  json.RawMessage `json:"command"`
						Output   string          `json:"aggregated_output"`
						ExitCode *int            `json:"exit_code"`
						Result   struct {
							IsError           bool            `json:"isError"`
							StructuredContent json.RawMessage `json:"structuredContent"`
						} `json:"result"`
					} `json:"item"`
				}
				if err := json.Unmarshal(event.Payload, &payload); err != nil {
					return err
				}
				// Nested shell calls may finish before the surrounding functions.exec
				// returns. Use their runtime results to establish ordering accurately.
				if payload.Type == "item_completed" && payload.Item.ExitCode != nil && *payload.Item.ExitCode == 0 {
					for _, match := range skillPath.FindAllStringSubmatch(string(payload.Item.Command), -1) {
						if strings.Contains(payload.Item.Output, "name: "+match[1]) {
							reads[match[1]] = true
						}
					}
				}
				if payload.Type == "item_completed" && payload.Item.Server == "edict-mcp" {
					if payload.Item.Tool == "edict_task_get" && !payload.Item.Result.IsError && !started {
						if err := json.Unmarshal(payload.Item.Result.StructuredContent, &setup.Assignment); err != nil {
							return err
						}
					}
					if payload.Item.Tool == "edict_task_start" && !started {
						started = true
						setup.SkillsAtStart = maps.Clone(reads)
						setup.FetchedBeforeStart = setup.Assignment.TaskID != ""
					}
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
		loaded[identity.id] = setup
		if identity.path != "" {
			loaded[identity.path] = setup
		}
		return scanner.Err()
	})
	return loaded, err
}

func TestManagedWorkerSetupRequiresFetchBeforeStartup(t *testing.T) {
	assignment := managed.TaskAssignment{TaskID: "task-one", Skill: "edict-next-signal-analysis", SkillPath: "managed-edict-next-signal-analysis/SKILL.md", Prompt: "$managed-edict-next-signal-analysis\nInspect commit-one."}
	for _, tc := range []struct {
		name  string
		calls []string
		valid bool
	}{
		{"fetch before start", []string{"edict_task_get", "edict_task_start"}, true},
		{"fetch after start", []string{"edict_task_start", "edict_task_get"}, false},
		{"no fetch", []string{"edict_task_start"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(home, "sessions"), 0o700))
			file, err := os.Create(filepath.Join(home, "sessions", "worker.jsonl"))
			require.NoError(t, err)
			encoder := json.NewEncoder(file)
			require.NoError(t, encoder.Encode(map[string]any{"type": "session_meta", "payload": map[string]any{"id": "worker", "source": "exec"}}))
			require.NoError(t, encoder.Encode(map[string]any{"type": "event_msg", "payload": map[string]any{
				"type": "item_completed", "item": map[string]any{"command": []string{"cat", "/skills/" + assignment.SkillPath},
					"aggregated_output": "---\nname: managed-" + assignment.Skill + "\n---", "exit_code": 0},
			}}))
			for _, tool := range tc.calls {
				require.NoError(t, encoder.Encode(map[string]any{"type": "event_msg", "payload": map[string]any{
					"type": "item_completed", "item": map[string]any{"type": "McpToolCall", "server": "edict-mcp", "tool": tool,
						"result": map[string]any{"structuredContent": assignment}},
				}}))
			}
			require.NoError(t, file.Close())
			setups, err := managedWorkerSkillReads(home)
			require.NoError(t, err)
			require.Equal(t, tc.valid, setups["worker"].FetchedBeforeStart)
			require.True(t, setups["worker"].SkillsAtStart["managed-"+assignment.Skill])
			if tc.valid {
				require.Equal(t, assignment, setups["worker"].Assignment)
			}
		})
	}
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
			require.Equal(t, tc.read, loaded["worker"].Skills[skill])
			require.Equal(t, loaded["worker"], loaded["/root/worker"])
			require.False(t, loaded["worker"].Skills["edict_manager"])
		})
	}
}
