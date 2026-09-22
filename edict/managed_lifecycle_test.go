// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package edict

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/edict/managed"
	"github.com/stretchr/testify/require"
)

// Check every recorded plan snapshot: the final completed plan alone cannot
// prove that stages ran in order or that each stage actually started.
func managedStageOrderProblems(input io.Reader, stages []string) ([]string, error) {
	var problems []string
	started, reported := make(map[string]bool), make(map[string]bool)
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	for line := 1; scanner.Scan(); line++ {
		var event struct {
			Message string `json:"msg"`
			Result  struct {
				StructuredContent struct {
					Tasks []managed.Task `json:"tasks"`
					Plan  *managed.Plan  `json:"plan"`
				} `json:"structuredContent"`
			} `json:"result"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("MCP log line %d: %w", line, err)
		}
		if event.Message != "response" {
			continue
		}
		snapshot := event.Result.StructuredContent
		if snapshot.Plan != nil {
			snapshot.Tasks = snapshot.Plan.Tasks
		}
		if len(snapshot.Tasks) == 0 {
			continue
		}
		states := make(map[string]string)
		for _, task := range snapshot.Tasks {
			states[task.ID] = task.Status
			if task.Status == "running" {
				started[task.ID] = true
			}
		}
		for i, id := range stages {
			if states[id] == "" || states[id] == "pending" {
				continue
			}
			for _, previous := range stages[:i] {
				if states[previous] != "completed" && !reported[id] {
					problems = append(problems, fmt.Sprintf("MCP log line %d: stage %s is %s before preceding stage %s completed (status %q)",
						line, id, states[id], previous, states[previous]))
					reported[id] = true
				}
			}
		}
	}
	for _, id := range stages {
		if !started[id] {
			problems = append(problems, fmt.Sprintf("stage %s has no recorded running state", id))
		}
	}
	return problems, scanner.Err()
}

func TestManagedStageOrderRejectsOverlapHiddenByCompletedPlan(t *testing.T) {
	ids := []string{"extract", "cluster", "generate"}
	snapshot := func(statuses ...string) string {
		var tasks []managed.Task
		for i, status := range statuses {
			tasks = append(tasks, managed.Task{ID: ids[i], Status: status})
		}
		data, err := json.Marshal(map[string]any{"msg": "response", "result": map[string]any{"structuredContent": managed.Plan{Tasks: tasks}}})
		require.NoError(t, err)
		return string(data) + "\n"
	}
	finished := snapshot("completed", "completed", "completed")
	for name, trace := range map[string]string{
		"overlap": snapshot("running", "running", "pending") + snapshot("completed", "completed", "running") + finished,
		"out-of-order": snapshot("pending", "running", "pending") + snapshot("running", "completed", "pending") +
			snapshot("completed", "completed", "running") + finished,
		"missing-executions": finished,
	} {
		t.Run(name, func(t *testing.T) {
			problems, err := managedStageOrderProblems(strings.NewReader(trace), ids)
			require.NoError(t, err)
			require.NotEmpty(t, problems, "successful final state must not conceal invalid stage execution")
		})
	}
	ordered := snapshot("running", "pending", "pending") + snapshot("completed", "running", "pending") +
		snapshot("completed", "completed", "running") + finished
	problems, err := managedStageOrderProblems(strings.NewReader(ordered), ids)
	require.NoError(t, err)
	require.Empty(t, problems)
}

func assertManagedLifecycle(t *testing.T, testRoot string) {
	t.Helper()
	file, err := os.Open(filepath.Join(testRoot, "log", "edict", "edict-mcp-system.log"))
	require.NoError(t, err)
	defer file.Close()
	problems, err := managedLifecycleProblems(file)
	require.NoError(t, err)
	for _, problem := range problems {
		t.Error(problem)
	}
}

// Inspect the history, not just the final plan: cancellation, failed completion,
// or rejected lifecycle calls must fail the happy-path test even after recovery.
func managedLifecycleProblems(input io.Reader) ([]string, error) {
	type call struct {
		Name      string `json:"name"`
		Arguments struct {
			Status string `json:"status"`
			Result string `json:"result"`
		} `json:"arguments"`
	}
	pending := make(map[int]call)
	var problems []string
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	for line := 1; scanner.Scan(); line++ {
		var event struct {
			Message   string `json:"msg"`
			Method    string `json:"method"`
			RequestID int    `json:"requestId"`
			Params    call   `json:"params"`
			Result    struct {
				IsError bool `json:"isError"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("MCP log line %d: %w", line, err)
		}
		if event.Method != "tools/call" {
			continue
		}
		if event.Message == "request" {
			name := event.Params.Name
			if name != "edict_delegate" && name != "edict_plan_create" && !strings.HasPrefix(name, "edict_task_") {
				continue
			}
			pending[event.RequestID] = event.Params
			if name == "edict_task_cancel" || (name == "edict_task_finish" && event.Params.Arguments.Status != "completed") {
				problems = append(problems, fmt.Sprintf("MCP log line %d: %s recorded task failure: %s", line, name, event.Params.Arguments.Result))
			}
		} else if event.Message == "response" {
			request, ok := pending[event.RequestID]
			if !ok {
				continue
			}
			delete(pending, event.RequestID)
			if event.Error != "" || event.Result.IsError {
				detail := event.Error
				for _, content := range event.Result.Content {
					detail += " " + content.Text
				}
				problems = append(problems, fmt.Sprintf("MCP log line %d: %s failed: %s", line, request.Name, strings.TrimSpace(detail)))
			}
		}
	}
	for id, request := range pending {
		problems = append(problems, fmt.Sprintf("%s request %d has no response", request.Name, id))
	}
	return problems, scanner.Err()
}

func TestManagedLifecycleRejectsFailuresHiddenBySuccessfulRetries(t *testing.T) {
	for _, failed := range []string{
		`{"msg":"request","method":"tools/call","requestId":1,"params":{"name":"edict_task_cancel","arguments":{"result":"Worker reported prompt-verification mismatch before task start"}}}
{"msg":"response","method":"tools/call","requestId":1,"result":{}}`,
		`{"msg":"request","method":"tools/call","requestId":1,"params":{"name":"edict_task_start"}}
{"msg":"response","method":"tools/call","requestId":1,"result":{"isError":true,"content":[{"text":"task prompt mismatch"}]}}`,
		`{"msg":"request","method":"tools/call","requestId":1,"params":{"name":"edict_task_finish","arguments":{"status":"failed","result":"Wrong task assignment"}}}
{"msg":"response","method":"tools/call","requestId":1,"result":{}}`,
	} {
		// The same task subsequently completes, which used to hide its failure.
		success := `{"msg":"request","method":"tools/call","requestId":2,"params":{"name":"edict_task_finish","arguments":{"status":"completed"}}}
{"msg":"response","method":"tools/call","requestId":2,"result":{}}`
		problems, err := managedLifecycleProblems(strings.NewReader(failed + "\n" + success))
		require.NoError(t, err)
		require.Len(t, problems, 1)
		problems, err = managedLifecycleProblems(strings.NewReader(success))
		require.NoError(t, err)
		require.Empty(t, problems)
	}
}
