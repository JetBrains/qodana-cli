/*
 * Copyright 2021-2024 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	edictmcp "github.com/JetBrains/qodana-cli/edict/mcp"
)

func TestMCPEndpointDetectorPublishesStreamableEndpoint(t *testing.T) {
	readyFile := t.TempDir() + "/ready.json"
	logOutput := &bytes.Buffer{}
	detector := &mcpEndpointDetector{log: logOutput, readyFile: readyFile}

	fragments := []string{
		"MCP server is running on port 64342:\n  - SSE endpoint: http://127.0.0.1:64342/sse\n",
		"  - Streamable HTTP end",
		"point: http://127.0.0.1:64342/mcp\x1b[0m\nTerminate the process\n",
	}
	for _, fragment := range fragments {
		if _, err := detector.Write([]byte(fragment)); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(readyFile)
	if err != nil {
		t.Fatal(err)
	}
	var ready edictmcp.Ready
	if err := json.Unmarshal(data, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Status != "ready" || ready.URL != "http://127.0.0.1:64342/mcp" {
		t.Fatalf("unexpected readiness: %+v", ready)
	}
	for _, fragment := range fragments {
		if !bytes.Contains(logOutput.Bytes(), []byte(fragment)) {
			t.Fatalf("log does not contain %q: %s", fragment, logOutput.String())
		}
	}
}
