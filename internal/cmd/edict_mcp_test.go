/*
 * Copyright 2021-2024 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	edictmcp "github.com/JetBrains/qodana-cli/edict/mcp"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/spf13/cobra"
)

func TestEdictMCPStartDefaultsToQodanaDistribution(t *testing.T) {
	t.Setenv(qdenv.QodanaDistEnv, "/opt/idea")
	command := newEdictMCPStartCommand(edictmcp.Service{})

	ide, err := command.Flags().GetString("ide")
	if err != nil {
		t.Fatal(err)
	}
	if ide != "/opt/idea" {
		t.Fatalf("expected IDE from %s, got %q", qdenv.QodanaDistEnv, ide)
	}
}

func TestEdictMCPStatusDefaultsToTabularOutput(t *testing.T) {
	command := newEdictMCPStatusCommand(edictmcp.Service{})
	output, err := command.Flags().GetString("output")
	if err != nil {
		t.Fatal(err)
	}
	if output != "tabular" {
		t.Fatalf("expected tabular output by default, got %q", output)
	}
}

func TestEdictMCPStatusWithoutState(t *testing.T) {
	stateFile := t.TempDir() + "/missing.json"
	service := edictmcp.Service{
		Processes: edictmcp.OSProcessController{},
	}
	command := newEdictMCPCommandWithService(service)
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetArgs([]string{"status", "--project-dir", t.TempDir(), "--state-file", stateFile, "--output", "json"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"status":"stopped"`) {
		t.Fatalf("unexpected status output: %s", output.String())
	}
}

func TestEdictMCPStatusTable(t *testing.T) {
	output := &bytes.Buffer{}
	status := edictmcp.StatusResult{
		Status:    "running",
		StateFile: "/tmp/mcp.json",
		State: edictmcp.State{
			PID:        123,
			URL:        "http://127.0.0.1:64342/mcp",
			ProjectDir: "/work/project",
			StartedAt:  time.Date(2026, time.August, 19, 12, 30, 0, 0, time.Local),
			LogFile:    "/tmp/mcp.log",
			Linter:     "qodana-jvm",
		},
	}

	if err := writeMCPStatusTable(output, status); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Edict MCP server", "✓ running", "http://127.0.0.1:64342/mcp",
		"PID:", "123", "/work/project", "qodana-jvm", "/tmp/mcp.log", "/tmp/mcp.json",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("status output does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestEdictMCPStatusRejectsUnknownFormat(t *testing.T) {
	command := &cobra.Command{}
	for _, output := range []string{"yaml", "JSON"} {
		if err := writeMCPStatus(command, edictmcp.StatusResult{}, output); err == nil {
			t.Fatalf("expected %q to be rejected", output)
		}
	}
}

func TestEdictCommandIncludesMCP(t *testing.T) {
	command := newEdictCommand()
	for _, child := range command.Commands() {
		if child.Name() == "mcp" {
			return
		}
	}
	t.Fatal("edict mcp command is not registered")
}
