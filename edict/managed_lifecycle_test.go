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

	"github.com/stretchr/testify/require"
)

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
