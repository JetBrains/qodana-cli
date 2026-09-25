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
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Exercise the real embedded JAR and bundled JBR, with no system Java, model, or IDE.
func TestEdictSetupCodexUsesKotlinBundle(t *testing.T) {
	for _, location := range []string{"custom", "project", "codex-home"} {
		t.Run(location, func(t *testing.T) {
			root := t.TempDir()
			project := filepath.Join(root, "project with spaces")
			codexHome := filepath.Join(root, "codex home")
			t.Setenv("CODEX_HOME", codexHome)
			args := []string{"--project-dir", project}
			var destination string
			switch location {
			case "custom":
				destination = filepath.Join(root, "custom skills")
				args = append(args, "--managed", "--project", "--dest", destination)
			case "project":
				destination = filepath.Join(project, ".codex", "skills")
				args = append(args, "--project")
			case "codex-home":
				destination = filepath.Join(codexHome, "skills")
			}
			// Ensure installation updates existing files, without affecting unrelated skills.
			manager := filepath.Join(destination, "edict_manager", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(manager), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manager, []byte("outdated"), 0o600); err != nil {
				t.Fatal(err)
			}
			unrelated := filepath.Join(destination, "user-file")
			if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			command := newEdictInstallCommand()
			command.SetArgs(args)
			command.SetIn(strings.NewReader(""))
			var output, stderr bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&stderr)
			if err := command.Execute(); err != nil {
				t.Fatalf("install: %v\n%s", err, &stderr)
			}
			if names := strings.Fields(output.String()); len(names) != 13 || strings.Contains(output.String(), "next") {
				t.Fatalf("expected the 13 renamed Kotlin managed skills, got %q", output.String())
			}
			bundle := filepath.Join("..", "..", "edict", "kotlin", "src", "main", "resources", "skills")
			err := filepath.WalkDir(bundle, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				relative, err := filepath.Rel(bundle, path)
				if err != nil {
					return err
				}
				expected, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				actual, err := os.ReadFile(filepath.Join(destination, relative))
				if err != nil {
					return err
				}
				if !bytes.Equal(expected, actual) {
					t.Errorf("installed resource differs from Kotlin bundle: %s", relative)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(unrelated); err != nil || string(data) != "keep" {
				t.Fatal("unrelated file changed")
			}
		})
	}
}

func TestEdictCodexDefaultSkillsDirectory(t *testing.T) {
	home := t.TempDir()
	homeVar := "HOME"
	if runtime.GOOS == "windows" {
		homeVar = "USERPROFILE"
	}
	t.Setenv(homeVar, home)
	t.Setenv("CODEX_HOME", "")
	directory, err := codexSkillsDirectory(false, "ignored")
	if err != nil || directory != filepath.Join(home, ".codex", "skills") {
		t.Fatalf("default skills directory: %q, %v", directory, err)
	}
}

func TestEdictJVMPropagatesFailure(t *testing.T) {
	command := newEdictInstallCommand()
	blocked := filepath.Join(t.TempDir(), "not a directory")
	if err := os.WriteFile(blocked, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	command.SetArgs([]string{"--dest", blocked})
	command.SetIn(strings.NewReader(""))
	var output, stderr bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&stderr)
	err := command.Execute()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("expected JVM exit 1, got %v", err)
	}
	if output.Len() != 0 || !strings.Contains(stderr.String(), "edict:") {
		t.Fatal("JVM diagnostics did not stay on stderr")
	}
}

func TestEdictManagedMCPHTTPProxy(t *testing.T) {
	project := t.TempDir()
	state := filepath.Join(project, "custom state")
	logs := filepath.Join(project, "custom logs")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stderr, stderrWriter := io.Pipe()
	defer stderr.Close()
	command := newEdictManagedMCPStartCommand()
	command.SetArgs([]string{"--project-dir", project, "--state-dir", state, "--log-dir", logs, "--http-port", "0"})
	command.SetIn(strings.NewReader(""))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(stderrWriter)
	done := make(chan error, 1)
	go func() {
		err := command.ExecuteContext(ctx)
		_ = stderrWriter.Close()
		done <- err
	}()
	endpoint := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if url, ok := strings.CutPrefix(scanner.Text(), "edict-mcp listening at "); ok {
				endpoint <- url
			}
		}
	}()
	var url string
	select {
	case url = <-endpoint:
	case err := <-done:
		t.Fatalf("HTTP server exited before startup: %v", err)
	case <-ctx.Done():
		t.Fatal("HTTP server startup timed out")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"edict_registry","arguments":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !json.Valid(body) || !bytes.Contains(body, []byte(`"edict-run"`)) || bytes.Contains(body, []byte("edict-next-")) {
		t.Fatalf("unexpected Kotlin HTTP registry response (status %d)", response.StatusCode)
	}
	// The forwarded state path is exclusively locked by the JVM.
	duplicate := newEdictManagedMCPStartCommand()
	duplicate.SetArgs([]string{"--project-dir", project, "--state-dir", state})
	duplicate.SetIn(strings.NewReader(""))
	duplicate.SetOut(&bytes.Buffer{})
	var duplicateErr bytes.Buffer
	duplicate.SetErr(&duplicateErr)
	if err := duplicate.ExecuteContext(ctx); err == nil || !strings.Contains(duplicateErr.String(), "already owned") {
		t.Fatalf("second JVM did not respect the state lock: %v", err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("HTTP shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("HTTP JVM did not stop after cancellation")
	}
	if output.Len() != 0 {
		t.Fatal("HTTP launch contaminated stdout")
	}
	if _, err := os.Stat(filepath.Join(logs, "edict", "edict-mcp-system.log")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project, ".edict")); !os.IsNotExist(err) {
		t.Fatal("custom state directory was ignored")
	}
	// A fresh process must be able to acquire the same state after cancellation.
	restart := newEdictManagedMCPStartCommand()
	restart.SetArgs([]string{"--project-dir", project, "--state-dir", state})
	restart.SetIn(strings.NewReader(""))
	restart.SetOut(&bytes.Buffer{})
	restart.SetErr(&bytes.Buffer{})
	if err := restart.Execute(); err != nil {
		t.Fatalf("state lock leaked after HTTP shutdown: %v", err)
	}
}
