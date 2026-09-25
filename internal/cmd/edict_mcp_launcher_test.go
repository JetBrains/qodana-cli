/*
 * Copyright 2021-2024 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	edictmcp "github.com/JetBrains/qodana-cli/edict/mcp"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
)

func TestPrepareMCPScanContextRequiresDistribution(t *testing.T) {
	t.Setenv(qdenv.QodanaDistEnv, "")
	_, _, cleanup, err := prepareMCPScanContext(edictmcp.LaunchRequest{ProjectDir: t.TempDir()})
	cleanup()
	if err == nil || !strings.Contains(err.Error(), "MCP startup requires") {
		t.Fatalf("expected missing distribution error, got %v", err)
	}
}

func TestIDEStarterSelectsPortAndPublishesStreamEndpoint(t *testing.T) {
	for _, port := range []int{0, 54321} {
		args := strings.Join(ideMCPArguments("/opt/idea", edictmcp.LaunchRequest{ProjectDir: "/project with spaces", Port: port}), "\n")
		if strings.Contains(args, "--port=0") || (port != 0 && !strings.Contains(args, "--port=54321")) {
			t.Fatalf("invalid port arguments: %s", args)
		}
	}
	readyFile := filepath.Join(t.TempDir(), "ready.json")
	detector := &mcpEndpointDetector{log: &bytes.Buffer{}, readyFile: readyFile}
	_, err := detector.Write([]byte("* SSE URL: http://127.0.0.1:54321/sse\n"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(readyFile)
	if err != nil || !bytes.Contains(data, []byte(`"url":"http://127.0.0.1:54321/stream"`)) {
		t.Fatalf("unexpected IDE readiness: %s, %v", data, err)
	}
}

// A real subprocess verifies that the server retains stdout/stderr after the
// short-lived launcher exits, and that a stale endpoint is never reused.
func TestMCPProcessSurvivesLauncherExit(t *testing.T) {
	const helperEnv = "QODANA_TEST_MCP_LAUNCHER"
	if role := os.Getenv(helperEnv); role != "" {
		dir := os.Getenv("QODANA_TEST_MCP_DIR")
		if role == "server" {
			_, _ = os.Stdout.WriteString("Streamable HTTP endpoint: http://127.0.0.1:54321/mcp\n")
			for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
				if _, err := os.Stat(filepath.Join(dir, "continue")); err == nil {
					_, _ = os.Stderr.WriteString("server logged after launcher exit\n")
					os.Exit(0)
				}
				time.Sleep(10 * time.Millisecond)
			}
			os.Exit(1)
		}
		_ = os.Setenv(helperEnv, "server")
		request := edictmcp.LaunchRequest{ProjectDir: dir, LogFile: filepath.Join(dir, "server.log"), ReadyFile: filepath.Join(dir, "ready.json")}
		process, err := startMCPProcess([]string{os.Args[0], "-test.run=^TestMCPProcessSurvivesLauncherExit$"}, request, "", func() {})
		if err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
			if _, err := os.Stat(request.ReadyFile); err == nil {
				os.Exit(0)
			}
			time.Sleep(10 * time.Millisecond)
		}
		_ = process.Kill()
		t.Fatal("server never became ready")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "server.log")
	if err := os.WriteFile(logPath, []byte("Streamable HTTP endpoint: http://127.0.0.1:1/stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	launcher := exec.Command(os.Args[0], "-test.run=^TestMCPProcessSurvivesLauncherExit$")
	launcher.Env = append(os.Environ(), helperEnv+"=launcher", "QODANA_TEST_MCP_DIR="+dir)
	if output, err := launcher.CombinedOutput(); err != nil {
		t.Fatalf("launcher failed: %v: %s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(dir, "ready.json"))
	if err != nil || !bytes.Contains(data, []byte("http://127.0.0.1:54321/mcp")) {
		t.Fatalf("wrong readiness: %s, %v", data, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "continue"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		data, _ := os.ReadFile(logPath)
		if bytes.Contains(data, []byte("server logged after launcher exit")) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server lost its log when the launcher exited")
}

func TestComputeNativeMCPContextLoadsTokenWithoutVCSDiscovery(t *testing.T) {
	t.Setenv(qdenv.QodanaDistEnv, "")
	t.Setenv(qdenv.QodanaToken, "license-token")
	qdenv.InitializeQodanaGlobalEnv(qdenv.EmptyEnvProvider())
	projectDir := t.TempDir()

	commonCtx := computeNativeMCPContext(edictmcp.LaunchRequest{
		ProjectDir: projectDir,
		Linter:     "qodana-jvm",
	})

	if commonCtx.QodanaToken != "license-token" {
		t.Fatalf("MCP context did not retain the token required for licensing")
	}
	if commonCtx.RepositoryRoot != commonCtx.ProjectDir {
		t.Fatalf("MCP context unexpectedly discovered a VCS root: %q", commonCtx.RepositoryRoot)
	}
}

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
