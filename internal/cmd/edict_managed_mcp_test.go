/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JetBrains/qodana-cli/edict/managed"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestEdictManagedMCPServesProtocolWithoutBootstrapToken(t *testing.T) {
	t.Run("client-disconnect", func(t *testing.T) { exerciseEdictManagedMCP(t, false) })
	t.Run("server-cancellation", func(t *testing.T) { exerciseEdictManagedMCP(t, true) })
}

func exerciseEdictManagedMCP(t *testing.T, cancelServer bool) {
	t.Helper()
	project := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverPipe, clientPipe := net.Pipe()
	defer serverPipe.Close()
	defer clientPipe.Close()
	command := newEdictCommand()
	command.SetArgs([]string{"edict-mcp", "--project-dir", project})
	command.SetIn(serverPipe)
	command.SetOut(serverPipe)
	command.SetErr(&bytes.Buffer{})
	done := make(chan error, 1)
	go func() { done <- command.ExecuteContext(ctx) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "cli-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: clientPipe, Writer: clientPipe}, nil)
	if err != nil {
		t.Fatalf("CLI stdout did not serve valid MCP: %v", err)
	}
	defer session.Close()
	if _, err := os.Stat(filepath.Join(project, ".edict")); err != nil {
		t.Fatalf("default state directory was not created: %v", err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "edict_plan_create", Arguments: map[string]any{"request": "Check project history", "steps": []managed.Step{
			{Skill: "edict-next-run", Title: "Process existing signals"},
		}},
	})
	if err != nil || result.IsError {
		t.Fatalf("tokenless plan creation failed: %v, %+v", err, result)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var created managed.PlanCreation
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatal(err)
	}
	if len(created.Token) != 64 || created.Plan == nil {
		t.Fatal("missing manager capability or plan")
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "edict_delegate", Arguments: map[string]any{"token": created.Token, "taskId": created.Plan.Tasks[0].ID},
	})
	if err != nil || result.IsError {
		t.Fatalf("returned manager token was not accepted: %v, %+v", err, result)
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "edict_plan_create", Arguments: map[string]any{"request": "Second manager", "steps": []managed.Step{{Skill: "edict-next-run", Title: "Duplicate"}}},
	})
	if err != nil || !result.IsError {
		t.Fatalf("second plan creation was not rejected: %v, %+v", err, result)
	}
	if cancelServer {
		cancel()
	} else {
		_ = session.Close()
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CLI did not stop when its context or MCP connection closed")
	}
	logData, err := os.ReadFile(filepath.Join(project, "log", "edict", "edict-mcp-system.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"msg":"request"`, `"msg":"response"`, "edict_plan_create", "Check project history", "already succeeded"} {
		if !bytes.Contains(logData, []byte(want)) {
			t.Errorf("server log is missing %q", want)
		}
	}
	if bytes.Contains(logData, []byte(created.Token)) {
		t.Fatal("server log exposed the manager capability")
	}
	activity, err := os.ReadFile(filepath.Join(project, "log", "edict", "edict-mcp.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`Plan ready: "Check project history"`, `Delegated task "Process existing signals"`, "already succeeded"} {
		if !bytes.Contains(activity, []byte(want)) {
			t.Errorf("activity log is missing %q", want)
		}
	}
	for _, unwanted := range []string{created.Token, `"msg":"request"`, "requestId", "structuredContent"} {
		if bytes.Contains(activity, []byte(unwanted)) {
			t.Errorf("activity log contains a capability or protocol details")
		}
	}
}

func TestEdictManagedMCPNeedsNoLoggingParameter(t *testing.T) {
	command := newEdictManagedMCPCommand()
	command.SetArgs([]string{"--project-dir", t.TempDir()})
	command.SetIn(strings.NewReader(""))
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	if err := command.Execute(); err != nil {
		t.Fatalf("startup without a logging parameter: %v", err)
	}
	if command.Flags().Lookup("log-dir") != nil {
		t.Fatal("unexpected log-dir flag")
	}
}

func TestEdictManagedMCPStartupFailureKeepsStdoutClean(t *testing.T) {
	directory := t.TempDir()
	state := filepath.Join(directory, "not-a-directory")
	if err := os.WriteFile(state, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := newEdictManagedMCPCommand()
	command.SetArgs([]string{"--state-dir", state, "--project-dir", directory})
	command.SetIn(strings.NewReader(""))
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(&bytes.Buffer{})
	if err := command.Execute(); err == nil {
		t.Fatal("expected invalid state directory error")
	}
	if output.Len() != 0 {
		t.Fatalf("startup failure contaminated stdout: %s", output.String())
	}
}

func TestEdictSetupCodexManagedFlag(t *testing.T) {
	destination := t.TempDir()
	command := newEdictSetupCodexCommand()
	command.SetArgs([]string{"--managed", "--dest", destination})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, skill := range []string{"edict_manager", "managed-edict-next-run", "managed-edict-next-batch-signal-analysis"} {
		if _, err := os.Stat(filepath.Join(destination, skill, "SKILL.md")); err != nil {
			t.Errorf("managed skill %s was not installed: %v", skill, err)
		}
	}
}

func TestManagedMCPCommandDetection(t *testing.T) {
	root := newRootCommand()
	root.AddCommand(newEdictCommand())
	for _, args := range [][]string{
		{"edict", "edict-mcp"},
		{"--log-level", "debug", "edict", "edict-mcp"},
		{"edict", "--disable-update-checks", "edict-mcp", "--state-dir", "/tmp/edict-state"},
	} {
		if !isManagedMCPCommand(root, args) {
			t.Errorf("stdio MCP invocation was not detected: %v", args)
		}
	}
	for _, args := range [][]string{
		{"edict", "mcp", "start"},
		{"edict", "setup-codex", "--dest", "edict-mcp"},
		{"edict"},
	} {
		if isManagedMCPCommand(root, args) {
			t.Errorf("ordinary CLI command was mistaken for MCP: %v", args)
		}
	}
}
