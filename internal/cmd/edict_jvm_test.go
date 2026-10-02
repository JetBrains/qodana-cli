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
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Exercise the real embedded JAR and bundled JBR, with no system Java, model, or IDE.
func TestEdictSetupCodexUsesKotlinBundle(t *testing.T) {
	for _, location := range []string{"custom", "project", "project-dir", "codex-home"} {
		t.Run(location, func(t *testing.T) {
			root := t.TempDir()
			project := filepath.Join(root, "project with spaces")
			codexHome := filepath.Join(root, "codex home")
			t.Setenv("CODEX_HOME", codexHome)
			var args []string
			var destination string
			switch location {
			case "custom":
				destination = filepath.Join(root, "custom skills")
				args = []string{"--dest", destination}
			case "project":
				destination = filepath.Join(project, ".codex", "skills")
				args = []string{"--project", "--project-dir", project}
			case "project-dir":
				// An explicit project directory implies a project installation.
				destination = filepath.Join(project, ".codex", "skills")
				args = []string{"-i", project}
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
			bundled := bundledSkills(t)
			if installed := strings.Fields(output.String()); !slices.Equal(installed, slices.Sorted(maps.Keys(bundled))) {
				t.Fatalf("installed %q, want every bundled skill %q", installed, slices.Sorted(maps.Keys(bundled)))
			}
			// Bundled skills may be grouped in folders; each is installed flat under its name.
			for name, root := range bundled {
				err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
					if err != nil || entry.IsDir() {
						return err
					}
					relative, err := filepath.Rel(root, path)
					if err != nil {
						return err
					}
					expected, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					actual, err := os.ReadFile(filepath.Join(destination, name, relative))
					if err != nil {
						return err
					}
					if !bytes.Equal(expected, actual) {
						t.Errorf("installed resource differs from Kotlin bundle: %s/%s", name, relative)
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if data, err := os.ReadFile(unrelated); err != nil || string(data) != "keep" {
				t.Fatal("unrelated file changed")
			}
		})
	}
}

// bundledSkills maps each skill's declared name to its directory in the Kotlin resources.
func bundledSkills(t *testing.T) map[string]string {
	t.Helper()
	bundle := filepath.Join("..", "..", "edict", "kotlin", "src", "main", "resources", "skills")
	skills := map[string]string{}
	err := filepath.WalkDir(bundle, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.Name() != "SKILL.md" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if name, ok := strings.CutPrefix(line, "name: "); ok {
				skills[strings.TrimSpace(name)] = filepath.Dir(path)
				return nil
			}
		}
		return fmt.Errorf("%s declares no name", path)
	})
	if err != nil {
		t.Fatal(err)
	}
	return skills
}

func TestEdictCodexDefaultSkillsDirectory(t *testing.T) {
	home := t.TempDir()
	homeVar := "HOME"
	if runtime.GOOS == "windows" {
		homeVar = "USERPROFILE"
	}
	t.Setenv(homeVar, home)
	t.Setenv("CODEX_HOME", "")
	directory, err := edictInstallOptions{}.skillsDirectory()
	if err != nil || directory != filepath.Join(home, ".codex", "skills") {
		t.Fatalf("default skills directory: %q, %v", directory, err)
	}
}

func TestEdictInstallRejectsProjectAndCustomDestinationTogether(t *testing.T) {
	for _, project := range [][]string{{"--project"}, {"--project-dir", t.TempDir()}} {
		command := newEdictInstallCommand()
		command.SetArgs(append(project, "--dest", t.TempDir()))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "none of the others can be") {
			t.Fatalf("%v with --dest: expected mutually exclusive destinations, got %v", project, err)
		}
	}
}

func TestEdictInstallProjectDefaultsToCurrentDirectory(t *testing.T) {
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	directory, err := edictInstallOptions{Project: true, ProjectDir: "."}.skillsDirectory()
	if err != nil || directory != filepath.Join(working, ".codex", "skills") {
		t.Fatalf("project skills directory: %q, %v", directory, err)
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
	command.SetArgs([]string{
		"--project-dir", project, "--state-dir", state, "--log-dir", logs, "--http-port", "0",
	})
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
	// Speak Streamable HTTP like an agent host: the SDK client initializes and keeps the session.
	client := mcp.NewClient(&mcp.Implementation{Name: "cli-http-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: url, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatalf("connecting to the Kotlin HTTP server: %v", err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "edict_registry", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("edict_registry over HTTP failed: %v, %+v", err, result)
	}
	registry, err := json.Marshal(result.StructuredContent)
	if err != nil || !bytes.Contains(registry, []byte(`"edict_manager"`)) || !bytes.Contains(registry, []byte(`"edict-next-run"`)) {
		t.Fatalf("unexpected Kotlin HTTP registry: %s, %v", registry, err)
	}
	_ = session.Close()
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
