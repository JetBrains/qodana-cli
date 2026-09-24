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

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestEdictManagedMCPServesProtocolWithoutBootstrapToken(t *testing.T) {
	t.Run("client-disconnect", func(t *testing.T) { exerciseEdictManagedMCP(t, false) })
	t.Run("server-cancellation", func(t *testing.T) { exerciseEdictManagedMCP(t, true) })
}

func exerciseEdictManagedMCP(t *testing.T, cancelServer bool) {
	t.Helper()
	project := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		Name: "edict_plan_create", Arguments: map[string]any{"request": "Check project history", "steps": []map[string]string{
			{"skill": "edict-run", "title": "Process existing signals"},
		}},
	})
	if err != nil || result.IsError {
		t.Fatalf("tokenless plan creation failed: %v, %+v", err, result)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		Token string `json:"token"`
		Plan  *struct {
			Tasks []struct {
				ID string `json:"id"`
			} `json:"tasks"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatal(err)
	}
	if len(created.Token) != 64 || created.Plan == nil || len(created.Plan.Tasks) != 1 {
		t.Fatal("missing manager capability or plan")
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "edict_delegate", Arguments: map[string]any{
			"token": created.Token, "taskId": created.Plan.Tasks[0].ID,
			"prompt": "$managed-edict-run\nRead /skills/managed-edict-run/SKILL.md. Process existing signals.",
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("returned manager token was not accepted: %v, %+v", err, result)
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "edict_plan_create", Arguments: map[string]any{"request": "Second manager", "steps": []map[string]string{{"skill": "edict-run", "title": "Duplicate"}}},
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
	restart := newEdictManagedMCPCommand()
	restart.SetArgs([]string{"--project-dir", project})
	restart.SetIn(strings.NewReader(""))
	restart.SetOut(&bytes.Buffer{})
	restart.SetErr(&bytes.Buffer{})
	if err := restart.Execute(); err != nil {
		t.Fatalf("restart after shutdown: %v", err)
	}

	logData, err := os.ReadFile(filepath.Join(project, "log", "edict", "edict-mcp-system.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{" => ", "edict_plan_create", "Check project history", "already succeeded"} {
		if !bytes.Contains(logData, []byte(want)) {
			t.Errorf("server log is missing %q", want)
		}
	}
	agents, err := os.ReadFile(filepath.Join(project, "log", "edict", "edict-agents.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Task prompt assigned by edict_manager/-:", "Check project history", "edict_plan_create response:"} {
		if !bytes.Contains(agents, []byte(want)) {
			t.Errorf("agent log is missing %q", want)
		}
	}
	if bytes.Contains(agents, []byte(created.Token)) {
		t.Fatal("agent log exposed capability")
	}
	if bytes.Contains(logData, []byte(created.Token)) {
		t.Fatal("server log exposed the manager capability")
	}
	activity, err := os.ReadFile(filepath.Join(project, "log", "edict", "edict-mcp.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"edict_plan_create ok", "edict_delegate ok", "edict_plan_create failed"} {
		if !bytes.Contains(activity, []byte(want)) {
			t.Errorf("activity log is missing %q", want)
		}
	}
	for _, unwanted := range []string{created.Token, `"msg":"request"`, "requestId", "structuredContent"} {
		if bytes.Contains(activity, []byte(unwanted)) {
			t.Errorf("activity log contains a capability or protocol details")
		}
	}
	short, err := os.ReadFile(filepath.Join(project, "log", "edict", "edict-agent-short.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"edict_plan_create ok", "Task delegated by edict_manager/-", "edict_plan_create failed"} {
		if !bytes.Contains(short, []byte(want)) {
			t.Errorf("short agent log is missing %q", want)
		}
	}
	for _, unwanted := range []string{created.Token, "response:", "SKILL.md", "managerToken:"} {
		if bytes.Contains(short, []byte(unwanted)) {
			t.Errorf("short agent log contains %q", unwanted)
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
