//go:build !windows

/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const fakeServerReady = `echo "edict-mcp listening at http://127.0.0.1:27182/mcp"`

func TestDetachedEdictServerKeepsRunningAfterStartReturns(t *testing.T) {
	files, script := fakeEdictServer(t, fakeServerReady+"\nexec sleep 60")

	var output bytes.Buffer
	if err := startDetachedEdictServer(script, nil, files, &output); err != nil {
		t.Fatal(err)
	}

	expected := fmt.Sprintf(
		"edict-mcp listening at http://127.0.0.1:27182/mcp\nServer log: %s\nStop it with `qodana edict mcp stop`.\n", files.log,
	)
	if output.String() != expected {
		t.Fatalf("start output:\n%s\nexpected:\n%s", output.String(), expected)
	}
	if !processAlive(recordedPid(t, files)) {
		t.Fatal("the detached server did not keep running")
	}
}

func TestDetachedEdictServerFailsWithItsLogWhenItExitsBeforeServing(t *testing.T) {
	files, script := fakeEdictServer(t, "echo 'Edict is not installed'\nexit 3")

	err := startDetachedEdictServer(script, nil, files, &bytes.Buffer{})

	expected := fmt.Sprintf(
		"the Edict server exited before serving MCP: exit status 3; server log %s:\nEdict is not installed", files.log,
	)
	if err == nil || err.Error() != expected {
		t.Fatalf("start error: %v\nexpected: %s", err, expected)
	}
}

func TestDetachedEdictServerRefusesASecondServerForTheProject(t *testing.T) {
	files, script := fakeEdictServer(t, fakeServerReady+"\nexec sleep 60")
	if err := startDetachedEdictServer(script, nil, files, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	err := startDetachedEdictServer(script, nil, files, &bytes.Buffer{})

	expected := fmt.Sprintf(
		"an Edict server already runs for this project (pid %d); stop it with `qodana edict mcp stop`", recordedPid(t, files),
	)
	if err == nil || err.Error() != expected {
		t.Fatalf("second start error: %v\nexpected: %s", err, expected)
	}
}

func TestStopEdictServerTerminatesTheRecordedServer(t *testing.T) {
	files, script := fakeEdictServer(t, fakeServerReady+"\nexec sleep 60")
	if err := startDetachedEdictServer(script, nil, files, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	pid := recordedPid(t, files)

	var output bytes.Buffer
	if err := stopEdictServer(files, 10*time.Second, &output); err != nil {
		t.Fatal(err)
	}

	if output.String() != fmt.Sprintf("Stopped the Edict server (pid %d)\n", pid) || processAlive(pid) {
		t.Fatalf("stop output %q; server alive: %v", output.String(), processAlive(pid))
	}
	if _, err := os.Stat(files.record); !os.IsNotExist(err) {
		t.Fatalf("the server record was kept: %v", err)
	}
}

func TestStopEdictServerKillsTheSessionOfAServerThatIgnoresTermination(t *testing.T) {
	// The server ignores SIGTERM and has started a child, as the IDE is.
	files, script := fakeEdictServer(t, "trap '' TERM\nsleep 60 &\necho $! > \"$0.child\"\n"+fakeServerReady+"\nwhile :; do sleep 1; done")
	if err := startDetachedEdictServer(script, nil, files, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	pid := recordedPid(t, files)

	var output bytes.Buffer
	if err := stopEdictServer(files, time.Second, &output); err != nil {
		t.Fatal(err)
	}

	child := readPid(t, script+".child")
	if output.String() != fmt.Sprintf("Killed the Edict server (pid %d), which did not stop within 1s\n", pid) {
		t.Fatalf("stop output: %q", output.String())
	}
	if holdsAfterWaiting(func() bool { return processAlive(pid) || processAlive(child) }) {
		t.Fatalf("server alive: %v, its child alive: %v", processAlive(pid), processAlive(child))
	}
}

func TestStopEdictServerTreatsARecordOfAnExitedProcessAsNoServer(t *testing.T) {
	files, script := fakeEdictServer(t, fakeServerReady+"\nexec sleep 60")
	if err := startDetachedEdictServer(script, nil, files, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(recordedPid(t, files), syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	holdsAfterWaiting(func() bool { return processAlive(recordedPid(t, files)) })

	var output bytes.Buffer
	if err := stopEdictServer(files, time.Second, &output); err != nil {
		t.Fatal(err)
	}

	if output.String() != "No Edict server runs for this project\n" {
		t.Fatalf("stop output: %q", output.String())
	}
	if _, err := os.Stat(files.record); !os.IsNotExist(err) {
		t.Fatalf("the record of the exited server was kept: %v", err)
	}
}

func TestForegroundArgumentsRepeatTheFlagsAsGiven(t *testing.T) {
	command := newEdictManagedMCPStartCommandWithRunner(nil)
	if err := command.ParseFlags([]string{
		"--config", "custom.yaml", "--ide-property=-Xmx8g", "--ide-property", "-Dfoo=a,b", "--ide-wait-timeout", "90m",
	}); err != nil {
		t.Fatal(err)
	}

	arguments := foregroundArguments(command.Flags())

	expected := []string{
		"edict", "mcp", "start", "--foreground",
		"--config=custom.yaml", "--ide-property=-Xmx8g", "--ide-property=-Dfoo=a,b", "--ide-wait-timeout=1h30m0s",
	}
	if !slices.Equal(arguments, expected) {
		t.Fatalf("foreground arguments:\n%v\nexpected:\n%v", arguments, expected)
	}
}

// fakeEdictServer writes a shell script standing in for a foreground `edict mcp start`, and server files beside it.
// Cleanup kills whatever the script leaves running.
func fakeEdictServer(t *testing.T, body string) (edictServerFiles, string) {
	t.Helper()
	directory := t.TempDir()
	script := filepath.Join(directory, "server.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := edictServerFiles{log: filepath.Join(directory, "server.log"), record: filepath.Join(directory, "server.pid")}
	t.Cleanup(func() {
		if data, err := os.ReadFile(files.record); err == nil {
			if pid, err := strconv.Atoi(strings.Fields(string(data))[0]); err == nil {
				_ = killProcessSession(pid)
			}
		}
	})
	return files, script
}

func recordedPid(t *testing.T, files edictServerFiles) int {
	t.Helper()
	return readPid(t, files.record)
}

// readPid reads the first number in path, a pid file or a server record.
func readPid(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.Fields(string(data))[0])
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// holdsAfterWaiting reports whether condition still holds after two seconds of waiting for it to stop holding.
func holdsAfterWaiting(condition func() bool) bool {
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if !condition() {
			return false
		}
	}
	return condition()
}
