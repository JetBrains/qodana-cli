/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
)

const fakeIDEEnv = "QODANA_TEST_FAKE_IDE"

// TestMain lets the test binary act as a fake IDE MCP server for serveIDEMCP, or as the fake Codex of useFakeCodex.
func TestMain(m *testing.M) {
	if os.Getenv(fakeCodexEnv) != "" {
		os.Exit(runFakeCodex())
	}
	switch os.Getenv(fakeIDEEnv) {
	case "":
		os.Exit(m.Run())
	case "serve":
		terminated := make(chan os.Signal, 1)
		signal.Notify(terminated, syscall.SIGTERM)
		_, _ = os.Stdout.WriteString("Starting IDE\n")
		_, _ = os.Stderr.WriteString("  - Streamable HTTP endpoint: http://127.0.0.1:54321/mcp\x1b[0m\n")
		<-terminated
		_, _ = os.Stderr.WriteString("fake IDE stopped\n")
		os.Exit(0)
	case "crash":
		_, _ = os.Stderr.WriteString("fake IDE failed\n")
		os.Exit(3)
	case "silent":
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}

func TestSelectIDEDistributionPrefersExplicitChoiceOverQodanaDist(t *testing.T) {
	for _, test := range []struct {
		name    string
		env     string
		options ideMCPOptions
		want    string
	}{
		{"dist overrides env", "/env-dist", ideMCPOptions{Dist: "/flag-dist"}, "/flag-dist"},
		{"linter overrides env", "/env-dist", ideMCPOptions{Linter: "qodana-jvm"}, ""},
		{"env by default", "/env-dist", ideMCPOptions{}, "/env-dist"},
		// The IDE runs in the project directory, so relative paths resolve from the caller's directory first.
		{"relative dist", "", ideMCPOptions{Dist: "../dist"}, filepath.Join(filepath.Dir(workingDirectory(t)), "dist")},
		{"relative env", "../env-dist", ideMCPOptions{}, filepath.Join(filepath.Dir(workingDirectory(t)), "env-dist")},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(qdenv.QodanaDistEnv, test.env)
			if err := selectIDEDistribution(test.options); err != nil {
				t.Fatal(err)
			}
			if got := os.Getenv(qdenv.QodanaDistEnv); got != test.want {
				t.Fatalf("%s = %q, want %q", qdenv.QodanaDistEnv, got, test.want)
			}
		})
	}
	t.Setenv(qdenv.QodanaDistEnv, "")
	if err := selectIDEDistribution(ideMCPOptions{}); err == nil || !strings.Contains(err.Error(), "requires --dist, --linter, or QODANA_DIST") {
		t.Fatalf("expected missing distribution error, got %v", err)
	}
}

// The Kotlin Edict server calls or forwards exactly these IntelliJ tools; keep both sides in sync.
func TestIDEStarterAllowsInspectionKtsTools(t *testing.T) {
	want := "--allowed-tools=generate_psi_tree,generate_inspection_kts_api,generate_inspection_kts_examples," +
		"run_inspection_kts,compile_inspection_kts,run_inspection_kts_examples,run_inspection_kts_project"
	if !slices.Contains(ideMCPArguments("/opt/idea", "/project"), want) {
		t.Fatalf("IDE MCP arguments do not allow the Edict inspection tools")
	}
}

func TestMCPEndpointRecognizesIDEAnnouncements(t *testing.T) {
	for line, want := range map[string]string{
		"  - Streamable HTTP endpoint: http://127.0.0.1:64342/mcp\x1b[0m\n": "http://127.0.0.1:64342/mcp",
		"* SSE URL: http://127.0.0.1:54321/sse\n":                           "http://127.0.0.1:54321/stream",
		"  - SSE endpoint: http://127.0.0.1:64342/sse\n":                    "",
	} {
		got, ok := mcpEndpoint(line)
		if got != want || ok != (want != "") {
			t.Errorf("mcpEndpoint(%q) = %q, %v; want %q", line, got, ok, want)
		}
	}
}

func TestPrepareMCPScanContextRejectsDockerLinters(t *testing.T) {
	t.Setenv(qdenv.QodanaDistEnv, "")
	qdenv.InitializeQodanaGlobalEnv(qdenv.EmptyEnvProvider())
	// A linter given as an image name resolves to Docker, which cannot read the host paths Edict passes.
	_, cleanup, err := prepareMCPScanContext(ideMCPOptions{ProjectDir: t.TempDir(), Linter: product.JvmLinter.Image()})
	cleanup()
	if err == nil || !strings.Contains(err.Error(), "Docker linters are not supported") {
		t.Fatalf("expected Docker rejection, got %v", err)
	}
}

func TestComputeNativeMCPContextLoadsTokenWithoutVCSDiscovery(t *testing.T) {
	t.Setenv(qdenv.QodanaDistEnv, "")
	t.Setenv(qdenv.QodanaToken, "license-token")
	qdenv.InitializeQodanaGlobalEnv(qdenv.EmptyEnvProvider())
	projectDir := t.TempDir()

	commonCtx := computeNativeMCPContext(ideMCPOptions{ProjectDir: projectDir, Linter: "qodana-jvm"})

	if commonCtx.QodanaToken != "license-token" {
		t.Fatalf("MCP context did not retain the token required for licensing")
	}
	if commonCtx.RepositoryRoot != commonCtx.ProjectDir {
		t.Fatalf("MCP context unexpectedly discovered a VCS root: %q", commonCtx.RepositoryRoot)
	}
}

func TestServeIDEMCPPublishesReadinessAndStopsIDEWhenStdinCloses(t *testing.T) {
	skipWithoutSignals(t)
	stdin, closeStdin := io.Pipe()
	stdout, stdoutWriter := io.Pipe()
	stderr := &syncBuffer{}
	done := make(chan error, 1)
	go func() {
		done <- serveIDEMCP(context.Background(), fakeIDE(t, "serve"), t.TempDir(), time.Minute, stdin, stdoutWriter, stderr)
		_ = stdoutWriter.Close()
	}()

	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("no readiness line: %v", err)
	}
	var ready struct {
		Status string
		URL    string
		PID    int
	}
	if err := json.Unmarshal([]byte(line), &ready); err != nil || ready.Status != "ready" || ready.URL != "http://127.0.0.1:54321/mcp" || ready.PID <= 0 {
		t.Fatalf("unexpected readiness %q: %v", line, err)
	}
	go func() { _, _ = io.Copy(io.Discard, stdout) }()

	_ = closeStdin.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stopping on stdin EOF failed: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("helper did not stop the IDE after stdin closed")
	}
	stderr.waitFor(t, "Starting IDE", "fake IDE stopped")
}

func TestServeIDEMCPFailsWhenIDEExitsBeforeReady(t *testing.T) {
	stdout := &syncBuffer{}
	err := serveIDEMCP(context.Background(), fakeIDE(t, "crash"), t.TempDir(), time.Minute, blockingReader(t), stdout, &syncBuffer{})
	if err == nil || !strings.Contains(err.Error(), "exited before becoming ready") {
		t.Fatalf("expected early exit error, got %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("failed startup wrote to stdout: %q", stdout.String())
	}
}

func TestServeIDEMCPStopsIDEThatNeverBecomesReady(t *testing.T) {
	skipWithoutSignals(t)
	started := time.Now()
	err := serveIDEMCP(context.Background(), fakeIDE(t, "silent"), t.TempDir(), 200*time.Millisecond, blockingReader(t), &syncBuffer{}, &syncBuffer{})
	if err == nil || !strings.Contains(err.Error(), "did not become ready within 200ms") {
		t.Fatalf("expected readiness timeout, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("timed-out IDE was not terminated promptly: %s", elapsed)
	}
}

func workingDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func fakeIDE(t *testing.T, role string) []string {
	t.Helper()
	t.Setenv(fakeIDEEnv, role)
	return []string{os.Args[0], "-test.run=^$"}
}

// blockingReader is a stdin that stays open for the whole test.
func blockingReader(t *testing.T) io.Reader {
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	return reader
}

func skipWithoutSignals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake IDE relies on SIGTERM")
	}
}

type syncBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *syncBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func (b *syncBuffer) waitFor(t *testing.T, fragments ...string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if text := b.String(); !slices.ContainsFunc(fragments, func(f string) bool { return !strings.Contains(text, f) }) {
			return
		}
	}
	t.Fatalf("output %q does not contain all of %q", b.String(), fragments)
}
