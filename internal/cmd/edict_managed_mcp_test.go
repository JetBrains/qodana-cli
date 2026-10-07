/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func TestEdictManagedMCPServesProtocolWithoutBootstrapToken(t *testing.T) {
	project := prepareEdictProject(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server := startEdictMCP(t, ctx)
	session := connectEdictMCP(t, ctx, server.url)
	defer session.Close()
	if _, err := os.Stat(filepath.Join(project, ".edict")); err != nil {
		t.Fatalf("default state directory was not created: %v", err)
	}
	result, err := session.CallTool(
		ctx, &mcp.CallToolParams{
			Name: "edict_plan_create", Arguments: map[string]any{
				"request": "Check project history", "steps": []map[string]string{
					{"skill": "edict-next-run", "title": "Process existing signals"},
				},
			},
		},
	)
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
	result, err = session.CallTool(
		ctx, &mcp.CallToolParams{
			Name: "edict_delegate", Arguments: map[string]any{
				"token": created.Token, "taskId": created.Plan.Tasks[0].ID,
				"prompt": "$edict-next-run\nRead /skills/edict-next-run/SKILL.md. Process existing signals.",
			},
		},
	)
	if err != nil || result.IsError {
		t.Fatalf("returned manager token was not accepted: %v, %+v", err, result)
	}
	result, err = session.CallTool(
		ctx, &mcp.CallToolParams{
			Name: "edict_plan_create",
			Arguments: map[string]any{
				"request": "Second manager",
				"steps":   []map[string]string{{"skill": "edict-next-run", "title": "Duplicate"}},
			},
		},
	)
	if err != nil || !result.IsError {
		t.Fatalf("second plan creation was not rejected: %v, %+v", err, result)
	}
	_ = session.Close()
	server.stop(t, cancel)
	restartContext, restartCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer restartCancel()
	startEdictMCP(t, restartContext).stop(t, restartCancel)

	logData, err := os.ReadFile(firstEdictProcessLog(t, ".", "edict-mcp-system.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{" => ", "edict_plan_create", "Check project history", "already succeeded"} {
		if !bytes.Contains(logData, []byte(want)) {
			t.Errorf("server log is missing %q", want)
		}
	}
	agents, err := os.ReadFile(firstEdictProcessLog(t, ".", "edict-agents.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Task prompt assigned by edict_manager/-:",
		"Check project history",
		"edict_plan_create response:",
	} {
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
	activity, err := os.ReadFile(firstEdictProcessLog(t, ".", "edict-mcp.log"))
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
	short, err := os.ReadFile(firstEdictProcessLog(t, ".", "edict-agent-short.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"edict_plan_create ok",
		"Task delegated by edict_manager/-",
		"edict_plan_create failed",
	} {
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

func TestEdictManagedMCPRequiresInstall(t *testing.T) {
	t.Chdir(t.TempDir())
	command := newEdictManagedMCPStartCommand()
	command.SetIn(strings.NewReader(""))
	if output, err := executeEdictCommand(context.Background(), command); err == nil ||
		!strings.Contains(output, "run `qodana edict install` first") {
		t.Fatalf("startup without installation: %v, %s", err, output)
	}
}

func TestEdictManagedMCPForwardsStateDirectory(t *testing.T) {
	var forwarded []string
	command := newEdictManagedMCPStartCommandWithRunner(func(_ *cobra.Command, args ...string) error {
		forwarded = append([]string(nil), args...)
		return nil
	})
	command.SetArgs([]string{"--state-dir", "/state"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []string{"mcp", "--parent-pid", strconv.Itoa(os.Getpid())}
	if executable, err := os.Executable(); err == nil {
		want = append(want, "--qodana-executable", executable)
	}
	want = append(want, "--state-dir", "/state")
	if strings.Join(forwarded, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("forwarded arguments: %q, want %q", forwarded, want)
	}
}

func TestEdictManagedMCPRequiresInstallationForConfiguredPort(t *testing.T) {
	prepareEdictProject(t)
	writeEdictPort(t, freePort(t))
	command := newEdictManagedMCPStartCommand()
	command.SetIn(strings.NewReader(""))
	if output, err := executeEdictCommand(context.Background(), command); err == nil ||
		!strings.Contains(output, "re-run `qodana edict install`") {
		t.Fatalf("startup with a changed port: %v, %s", err, output)
	}
}

func TestEdictManagedMCPForwardsDefaultQodanaYamlPath(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	config := `version: "1.0"
edict:
  ci:
    url: https://github.com/JetBrains/qodana-cli
  promotion:
    reviewer: reviewer-login
    targetBranch: main
    inspectionsDirectory: quality/inspections
`
	if err := os.WriteFile(filepath.Join(project, "qodana.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	var forwarded []string
	command := newEdictManagedMCPStartCommandWithRunner(func(_ *cobra.Command, args ...string) error {
		forwarded = append([]string(nil), args...)
		return nil
	})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	expected := "--qodana-yaml\x00qodana.yaml"
	if !strings.Contains(strings.Join(forwarded, "\x00"), expected) {
		t.Errorf("forwarded arguments %q do not contain %q", forwarded, expected)
	}
}

func TestEdictManagedMCPForwardsCustomQodanaYamlWithoutParsing(t *testing.T) {
	project := t.TempDir()
	custom := filepath.Join(project, "config", "custom.yaml")
	if err := os.MkdirAll(filepath.Dir(custom), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(custom, []byte("edict:\n  ci:\n    url:\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var forwarded []string
	command := newEdictManagedMCPStartCommandWithRunner(func(_ *cobra.Command, args ...string) error {
		forwarded = append([]string(nil), args...)
		return nil
	})
	command.SetArgs([]string{"--config", custom})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(forwarded, "\x00"), "--qodana-yaml\x00"+custom) {
		t.Fatalf("Go launcher parsed or rejected custom YAML instead of forwarding its path: %q", forwarded)
	}
}

func TestEdictManagedMCPForwardsYamlWithoutEdictSection(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	if err := os.WriteFile(filepath.Join(project, "qodana.yaml"), []byte("version: \"1.0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var forwarded []string
	command := newEdictManagedMCPStartCommandWithRunner(func(_ *cobra.Command, args ...string) error {
		forwarded = append([]string(nil), args...)
		return nil
	})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(forwarded, "\x00"), "--qodana-yaml\x00qodana.yaml") {
		t.Fatalf("Qodana YAML path was not forwarded: %q", forwarded)
	}
}

func TestEdictManagedMCPFailsForInvalidStateDirectory(t *testing.T) {
	prepareEdictProject(t)
	if err := os.WriteFile("not-a-directory", []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := newEdictManagedMCPStartCommand()
	command.SetArgs([]string{"--state-dir", "not-a-directory"})
	command.SetIn(strings.NewReader(""))
	if output, err := executeEdictCommand(context.Background(), command); err == nil || !strings.Contains(output, "ERROR") {
		t.Fatalf("startup with an invalid state directory: %v, %s", err, output)
	}
}

func TestEdictServerCommandDetection(t *testing.T) {
	root := newRootCommand()
	root.AddCommand(newEdictCommand())
	for _, args := range [][]string{
		{"edict", "mcp", "start"},
		{"--log-level", "debug", "edict", "mcp", "start"},
		{"edict", "--disable-update-checks", "mcp", "start", "--state-dir", "/tmp/edict-state"},
		{"edict", "ide-mcp", "--project-dir", "/tmp/project"},
	} {
		if !isEdictServerCommand(root, args) {
			t.Errorf("Edict server invocation was not detected: %v", args)
		}
	}
	for _, args := range [][]string{
		{"edict", "install", "--deny", "mcp"},
		{"edict", "mcp"},
		{"edict"},
	} {
		if isEdictServerCommand(root, args) {
			t.Errorf("ordinary CLI command was mistaken for an Edict server: %v", args)
		}
	}
}

func TestEdictIDEMCPCommandIsHidden(t *testing.T) {
	for _, command := range newEdictCommand().Commands() {
		if command.Name() == "ide-mcp" && !command.Hidden {
			t.Fatal("ide-mcp is internal to the Kotlin Edict server and must stay hidden")
		}
	}
}
