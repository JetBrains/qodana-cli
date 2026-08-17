/*
 * Copyright 2021-2024 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"bytes"
	"strings"
	"testing"

	edictmcp "github.com/JetBrains/qodana-cli/edict/mcp"
)

func TestEdictMCPStatusWithoutState(t *testing.T) {
	stateFile := t.TempDir() + "/missing.json"
	service := edictmcp.Service{
		Processes: edictmcp.OSProcessController{},
	}
	command := newEdictMCPCommandWithService(service)
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetArgs([]string{"status", "--project-dir", t.TempDir(), "--state-file", stateFile})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"status":"stopped"`) {
		t.Fatalf("unexpected status output: %s", output.String())
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
