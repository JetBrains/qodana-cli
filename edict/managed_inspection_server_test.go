// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package edict

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

type managedInspectionServer struct {
	URL         string
	client      *mcp.ClientSession
	projectPath string
}

// Run the real compiler against a disposable source copy. Expose only generic
// inspection tools, so the IDE cannot bypass managed state ownership.
func startManagedInspectionServer(t *testing.T, project managedTestProject, ultimate string) managedInspectionServer {
	t.Helper()
	root := project.Checkout.TestRoot
	source := filepath.Join(root, "inspection-project")
	require.NoError(t, os.CopyFS(source, os.DirFS(project.ProjectDirectory)))
	logPath := filepath.Join(root, "log", "intellij-mcp.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o700))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, logFile.Close()) })
	ctx, cancel := context.WithCancel(context.Background())
	allowed := []string{"generate_psi_tree", "generate_inspection_kts_api", "generate_inspection_kts_examples", "run_inspection_kts"}
	command := exec.CommandContext(ctx, "/bin/sh", "./bazel.cmd", "run", "//build:mcp_server", "--",
		"--jvm_flag=-Didea.config.path="+filepath.Join(root, "intellij", "config"),
		"--jvm_flag=-Didea.system.path="+filepath.Join(root, "intellij", "system"),
		"--jvm_flag=-Didea.log.path="+filepath.Join(root, "log", "intellij"),
		"mcpServer", "--project="+source, "--invocation-mode=direct", "--allowed-tools="+strings.Join(allowed, ","))
	command.Dir, command.Stdout, command.Stderr = ultimate, logFile, logFile
	// Bazel forwards SIGINT to the IDE it launched; killing only the Bazel
	// client would leave a server behind after the test.
	command.Cancel = func() error { return command.Process.Signal(os.Interrupt) }
	command.WaitDelay = 15 * time.Second
	if err := command.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan struct{})
	var processErr error
	go func() { processErr = command.Wait(); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	t.Logf("Starting IntelliJ MCP through Bazel; log: %s", logPath)
	endpoint := regexp.MustCompile(`(?:Streamable HTTP endpoint|SSE URL):\s*(https?://[^\s\x1b]+)`)
	deadline := time.NewTimer(10 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var url string
	for url == "" {
		select {
		case <-done:
			t.Fatalf("IntelliJ MCP exited during startup: %v; see %s", processErr, logPath)
		case <-deadline.C:
			t.Fatalf("IntelliJ MCP did not become ready within 10 minutes; see %s", logPath)
		case <-ticker.C:
			data := mustReadFile(t, logPath)
			if match := endpoint.FindSubmatch(data); len(match) == 2 {
				url = string(match[1])
			}
		}
	}
	connectCtx, stop := context.WithTimeout(ctx, time.Minute)
	defer stop()
	var transport mcp.Transport = &mcp.StreamableClientTransport{Endpoint: url}
	if strings.HasSuffix(url, "/sse") {
		transport = &mcp.SSEClientTransport{Endpoint: url}
	}
	// SSE's HTTP request retains this context for the session's lifetime.
	connectTimeout := time.AfterFunc(time.Minute, cancel)
	client, err := mcp.NewClient(&mcp.Implementation{Name: "managed-generation-test", Version: "1"}, nil).
		Connect(ctx, transport, nil)
	connectTimeout.Stop()
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	listed, err := client.ListTools(connectCtx, nil)
	require.NoError(t, err)
	proxy := mcp.NewServer(&mcp.Implementation{Name: "inspection", Version: "1"}, nil)
	audit, err := os.OpenFile(filepath.Join(root, "log", "inspection-mcp.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, audit.Close()) })
	var mu sync.Mutex
	var found []string
	for _, tool := range listed.Tools {
		if !slices.Contains(allowed, tool.Name) {
			continue
		}
		found = append(found, tool.Name)
		proxy.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var arguments map[string]any
			if err := json.Unmarshal(req.Params.Arguments, &arguments); err != nil {
				return nil, err
			}
			// The source project and its IDE snapshot represent the same files.
			// Workers keep using their assigned source path; only the proxy needs
			// to know which disposable copy the IDE has open.
			if path, _ := arguments["projectPath"].(string); path == "" || path == project.ProjectDirectory {
				arguments["projectPath"] = source
			}
			result, callErr := client.CallTool(ctx, &mcp.CallToolParams{Name: req.Params.Name, Arguments: arguments})
			entry := map[string]any{"tool": req.Params.Name, "arguments": arguments, "requestedArguments": req.Params.Arguments, "result": result}
			if callErr != nil {
				entry["error"] = callErr.Error()
			}
			mu.Lock()
			logErr := json.NewEncoder(audit).Encode(entry)
			mu.Unlock()
			if logErr != nil {
				return nil, fmt.Errorf("record inspection call: %w", logErr)
			}
			return result, callErr
		})
	}
	require.ElementsMatch(t, allowed, found, "IntelliJ must provide the generic inspection compiler and documentation tools")
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return proxy }, nil))
	t.Cleanup(httpServer.Close)
	return managedInspectionServer{URL: httpServer.URL, client: client, projectPath: source}
}

type managedInspectionResult struct {
	CompilationSuccess bool                       `json:"compilationSuccess"`
	CompilationStatus  string                     `json:"compilationStatus"`
	FoundProblems      []managedInspectionProblem `json:"foundProblems"`
}

type managedInspectionProblem struct {
	LineNumber int `json:"lineNumber"`
}

func decodeManagedInspectionResult(result *mcp.CallToolResult) (managedInspectionResult, error) {
	var output managedInspectionResult
	if result == nil || result.IsError {
		if result == nil {
			return output, fmt.Errorf("inspection MCP call returned no result")
		}
		return output, fmt.Errorf("inspection MCP call failed: %s", managedResultText(result))
	}
	if result.StructuredContent != nil {
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return output, err
		}
		err = json.Unmarshal(data, &output)
		return output, err
	}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok && json.Valid([]byte(text.Text)) {
			err := json.Unmarshal([]byte(text.Text), &output)
			return output, err
		}
	}
	return output, fmt.Errorf("inspection result has no JSON content")
}

func (server managedInspectionServer) run(t *testing.T, code, path, source string) managedInspectionResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, err := server.client.CallTool(ctx, &mcp.CallToolParams{Name: "run_inspection_kts", Arguments: map[string]any{
		"inspectionKtsCode": code, "contextPath": path, "targetFileContent": source, "projectPath": server.projectPath,
	}})
	require.NoError(t, err)
	output, err := decodeManagedInspectionResult(result)
	require.NoError(t, err)
	require.True(t, output.CompilationSuccess, "inspection compilation failed: %s", output.CompilationStatus)
	return output
}

func TestManagedInspectionResultDecodesRecordedCompilerOutput(t *testing.T) {
	for _, envelope := range []string{
		`{"content":[{"type":"text","text":"{\"compilationSuccess\":true,\"foundProblems\":[{\"lineNumber\":5}]}"}]}`,
		`{"content":[],"structuredContent":{"compilationSuccess":true,"foundProblems":[{"lineNumber":5}]}}`,
	} {
		var recorded mcp.CallToolResult
		require.NoError(t, json.Unmarshal([]byte(envelope), &recorded))
		result, err := decodeManagedInspectionResult(&recorded)
		require.NoError(t, err)
		require.True(t, result.CompilationSuccess)
		require.Equal(t, []managedInspectionProblem{{LineNumber: 5}}, result.FoundProblems)
	}
	_, err := decodeManagedInspectionResult(&mcp.CallToolResult{IsError: true})
	require.Error(t, err)
}
