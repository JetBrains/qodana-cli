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
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// Exercise the real embedded JAR and bundled JBR, with no system Java, model, or IDE.
func TestEdictInstallSetsUpCodexInCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project with spaces")
	codexHome := filepath.Join(root, "codex home")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	// Ensure installation updates existing files, without affecting unrelated skills.
	destination := filepath.Join(project, ".codex", "skills")
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
	// Resolve the bundle before leaving the package directory.
	bundled := bundledSkills(t)
	t.Chdir(project)
	project = realWorkingDirectory(t)
	useFakeCodex(t)
	t.Setenv("CODEX_HOME", codexHome)
	// Codex ignores the local config of an untrusted project; TestEdictInstallRequiresCodexTrust checks the real rule.
	ignored := filepath.Join(codexHome, fakeCodexAnswerFile)
	if err := os.WriteFile(ignored, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := runEdictInstall(); err == nil || !strings.Contains(output, "is not trusted") {
		t.Fatalf("install into a project Codex does not trust: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatal("failed install kept the local Codex config")
	}
	if err := os.Remove(ignored); err != nil {
		t.Fatal(err)
	}
	trusted := trustCodexProject(t, codexHome, project)
	if output := installEdict(t, "--deny", "gold.sarif.json"); !strings.Contains(output, "Installed skills") {
		t.Fatalf("install did not log the installed skills:\n%s", output)
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
	config := readCodexConfig(t)
	for _, want := range []string{
		`default_permissions = "edict"`,
		strconv.Quote(project) + ` = "read"`,
		strconv.Quote(filepath.Join(project, "log", "agent-work")) + ` = "write"`,
		strconv.Quote(filepath.Join(project, "log", "process-log")) + ` = "deny"`,
		strconv.Quote(filepath.Join(project, "gold.sarif.json")) + ` = "deny"`,
		"[mcp_servers.edict-mcp]",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("local Codex config is missing %s:\n%s", want, config)
		}
	}
	if url := edictMCPURL(t); url != "http://127.0.0.1:27182/mcp" {
		t.Fatalf("installation without edict.mcpPort did not use the default port: %s", url)
	}
	writeEdictPort(t, 4321)
	installEdict(t)
	if url := edictMCPURL(t); url != "http://127.0.0.1:4321/mcp" {
		t.Fatalf("re-installation did not follow edict.mcpPort: %s", url)
	}
	if strings.Contains(readCodexConfig(t), "gold.sarif.json") {
		t.Fatal("re-installation kept a deny rule that was not requested again")
	}
	// Codex's own runtime folders appear in CODEX_HOME when install asks it about the project; its config must not change.
	if config, err := os.ReadFile(filepath.Join(codexHome, "config.toml")); err != nil || string(config) != trusted {
		t.Fatalf("install changed the global Codex config: %v\n%s", err, config)
	}
}

// The contract the fake Codex stands in for: Codex loads .codex/config.toml only for a project trusted in CODEX_HOME.
func TestEdictInstallRequiresCodexTrust(t *testing.T) {
	codex, err := exec.LookPath("codex")
	if err != nil {
		// CI must run this check; elsewhere it needs Codex on PATH.
		if os.Getenv("CI") != "" || os.Getenv("TEAMCITY_VERSION") != "" {
			t.Fatalf("codex is not on PATH: %v", err)
		}
		t.Skip("codex is not on PATH")
	}
	t.Setenv("CODEX_BIN", codex)
	t.Chdir(t.TempDir())
	project := realWorkingDirectory(t)
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	if output, err := runEdictInstall(); err == nil || !strings.Contains(output, "is not trusted") {
		t.Fatalf("install into a project unknown to Codex: %v\n%s", err, output)
	}
	untrusted := fmt.Sprintf("[projects.%s]\ntrust_level = \"untrusted\"\n", strconv.Quote(project))
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(untrusted), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := runEdictInstall(); err == nil || !strings.Contains(output, "is not trusted") {
		t.Fatalf("install into a project marked untrusted: %v\n%s", err, output)
	}
	trustCodexProject(t, codexHome, project)
	installEdict(t)
}

const (
	fakeCodexEnv = "QODANA_TEST_FAKE_CODEX"
	// A test makes the fake Codex ignore the project config by writing another answer here, in CODEX_HOME.
	fakeCodexAnswerFile = "fake-mcp-list.json"
)

// useFakeCodex points CODEX_BIN at the test binary, which then answers Edict's trust check, `codex mcp list --json`,
// as if Codex loaded the project's edict-mcp server. It works without Codex and on every platform.
func useFakeCodex(t *testing.T) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_BIN", executable)
	t.Setenv(fakeCodexEnv, "1")
}

func runFakeCodex() int {
	if args := strings.Join(os.Args[1:], " "); args != "mcp list --json" {
		_, _ = fmt.Fprintf(os.Stderr, "fake codex supports only 'mcp list --json', got: %s\n", args)
		return 2
	}
	if answer, err := os.ReadFile(filepath.Join(os.Getenv("CODEX_HOME"), fakeCodexAnswerFile)); err == nil {
		_, _ = os.Stdout.Write(answer)
		return 0
	}
	_, _ = os.Stdout.WriteString(`[{"name":"edict-mcp"}]`)
	return 0
}

// installEdict runs `qodana edict install` in the current directory and returns its console output.
func installEdict(t *testing.T, args ...string) string {
	t.Helper()
	output, err := runEdictInstall(args...)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, output)
	}
	return output
}

func runEdictInstall(args ...string) (string, error) {
	command := newEdictInstallCommand()
	command.SetArgs(args)
	command.SetIn(strings.NewReader(""))
	return executeEdictCommand(context.Background(), command)
}

// executeEdictCommand runs command and returns its console output; the JVM logs to stdout, errors included.
func executeEdictCommand(ctx context.Context, command *cobra.Command) (string, error) {
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	err := command.ExecuteContext(ctx)
	return output.String(), err
}

// trustCodexProject points CODEX_HOME at codexHome and trusts project there, as the user does before installing Edict.
func trustCodexProject(t *testing.T, codexHome, project string) string {
	t.Helper()
	config := fmt.Sprintf("[projects.%s]\ntrust_level = \"trusted\"\n", strconv.Quote(project))
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", codexHome)
	return config
}

func readCodexConfig(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// edictMCPURL is the edict-mcp address that install wrote for the current directory.
func edictMCPURL(t *testing.T) string {
	t.Helper()
	for _, line := range strings.Split(readCodexConfig(t), "\n") {
		if value, ok := strings.CutPrefix(line, "url = "); ok {
			url, err := strconv.Unquote(value)
			if err != nil {
				t.Fatal(err)
			}
			return url
		}
	}
	t.Fatal("local Codex config has no edict-mcp url")
	return ""
}

// bundledSkills maps each skill's declared name to its directory in the Kotlin resources.
func bundledSkills(t *testing.T) map[string]string {
	t.Helper()
	// Absolute, so the returned directories stay valid after a test changes directory.
	bundle, err := filepath.Abs(filepath.Join("..", "..", "edict", "kotlin", "src", "main", "resources", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	skills := map[string]string{}
	err = filepath.WalkDir(bundle, func(path string, entry fs.DirEntry, err error) error {
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

func TestEdictJVMPropagatesFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	trustCodexProject(t, t.TempDir(), realWorkingDirectory(t))
	if err := os.WriteFile(".codex", []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := runEdictInstall()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("expected JVM exit 1, got %v", err)
	}
	if !strings.Contains(output, "ERROR") {
		t.Fatalf("JVM did not log the failure:\n%s", output)
	}
}

func TestEdictManagedMCPHTTPProxy(t *testing.T) {
	project := prepareEdictProject(t)
	state := filepath.Join(project, "custom state")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server := startEdictMCP(t, ctx, "--state-dir", state)
	if server.url != edictMCPURL(t) {
		t.Fatalf("server listens at %s, not at the installed address %s", server.url, edictMCPURL(t))
	}
	// Speak Streamable HTTP like an agent host: the SDK client initializes and keeps the session.
	session := connectEdictMCP(t, ctx, server.url)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "edict_registry", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("edict_registry over HTTP failed: %v, %+v", err, result)
	}
	registry, err := json.Marshal(result.StructuredContent)
	if err != nil || !bytes.Contains(registry, []byte(`"edict_manager"`)) || !bytes.Contains(registry, []byte(`"edict-next-run"`)) {
		t.Fatalf("unexpected Kotlin HTTP registry: %s, %v", registry, err)
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "edict_context", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("edict_context over HTTP failed: %v, %+v", err, result)
	}
	var paths struct{ ProjectDirectory, StateDirectory, ScratchDirectory string }
	if data, err := json.Marshal(result.StructuredContent); err != nil || json.Unmarshal(data, &paths) != nil {
		t.Fatalf("unexpected edict_context result: %+v", result)
	}
	if paths.ProjectDirectory != project || paths.StateDirectory != state ||
		!strings.HasPrefix(paths.ScratchDirectory, filepath.Join(project, "log", "agent-work")+string(filepath.Separator)) {
		t.Fatalf("unexpected run paths: %+v", paths)
	}
	if info, err := os.Stat(paths.ScratchDirectory); err != nil || !info.IsDir() {
		t.Fatalf("scratch directory was not created: %v", err)
	}
	_ = session.Close()
	// The forwarded state path is exclusively locked by the JVM.
	duplicate := newEdictManagedMCPStartCommand()
	duplicate.SetArgs([]string{"--state-dir", state})
	duplicate.SetIn(strings.NewReader(""))
	if output, err := executeEdictCommand(ctx, duplicate); err == nil || !strings.Contains(output, "already owned") {
		t.Fatalf("second JVM did not respect the state lock: %v, %s", err, output)
	}
	// Another state cannot move to another port: the agent config names this one.
	busy := newEdictManagedMCPStartCommand()
	busy.SetArgs([]string{"--state-dir", filepath.Join(project, "other state")})
	busy.SetIn(strings.NewReader(""))
	if output, err := executeEdictCommand(ctx, busy); err == nil || !strings.Contains(output, "Address already in use") {
		t.Fatalf("second JVM did not fail on the busy port: %v, %s", err, output)
	}
	server.stop(t, cancel)
	firstEdictProcessLog(t, project, "edict.log")
	firstEdictProcessLog(t, project, "edict-mcp-system.log")
	if _, err := os.Stat(filepath.Join(project, ".edict")); !os.IsNotExist(err) {
		t.Fatal("custom state directory was ignored")
	}
	// A fresh process must be able to acquire the same state and port after cancellation.
	restartContext, restartCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer restartCancel()
	startEdictMCP(t, restartContext, "--state-dir", state).stop(t, restartCancel)
}

// prepareEdictProject installs Edict into a fresh project and makes it the current directory.
func prepareEdictProject(t *testing.T) string {
	t.Helper()
	t.Chdir(t.TempDir())
	project := realWorkingDirectory(t)
	useFakeCodex(t)
	trustCodexProject(t, t.TempDir(), project)
	// TODO: drop once server startup no longer requires a Git repository (Main.kt GitRepository).
	if output, err := exec.Command("git", "init", "-q", project).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	// A free port keeps tests independent of a real server on the default port.
	writeEdictPort(t, freePort(t))
	installEdict(t)
	return project
}

func writeEdictPort(t *testing.T, port int) {
	t.Helper()
	if err := os.WriteFile("qodana.yaml", []byte(fmt.Sprintf("version: \"1.0\"\nedict:\n  mcpPort: %d\n", port)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

// realWorkingDirectory resolves symlinks such as macOS /var -> /private/var, like the Kotlin side does.
func realWorkingDirectory(t *testing.T) string {
	t.Helper()
	working, err := os.Getwd()
	if err == nil {
		working, err = filepath.EvalSymlinks(working)
	}
	if err != nil {
		t.Fatal(err)
	}
	return working
}

type edictMCPServer struct {
	url  string
	done chan error
}

// startEdictMCP runs `qodana edict mcp start` in the current directory until ctx is cancelled.
func startEdictMCP(t *testing.T, ctx context.Context, args ...string) edictMCPServer {
	t.Helper()
	output, outputWriter := io.Pipe()
	t.Cleanup(func() { _ = output.Close() })
	command := newEdictManagedMCPStartCommand()
	command.SetArgs(args)
	command.SetIn(strings.NewReader(""))
	server := edictMCPServer{done: make(chan error, 1)}
	command.SetOut(outputWriter)
	command.SetErr(outputWriter)
	go func() {
		err := command.ExecuteContext(ctx)
		_ = outputWriter.Close()
		server.done <- err
	}()
	endpoint := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			if _, url, ok := strings.Cut(scanner.Text(), "edict-mcp listening at "); ok {
				endpoint <- url
			}
		}
	}()
	select {
	case server.url = <-endpoint:
	case err := <-server.done:
		t.Fatalf("HTTP server exited before startup: %v", err)
	case <-ctx.Done():
		t.Fatal("HTTP server startup timed out")
	}
	return server
}

func (server edictMCPServer) stop(t *testing.T, cancel context.CancelFunc) {
	t.Helper()
	cancel()
	select {
	case err := <-server.done:
		if err != nil {
			t.Fatalf("HTTP shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("HTTP JVM did not stop after cancellation")
	}
}

func connectEdictMCP(t *testing.T, ctx context.Context, url string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "cli-http-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: url, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatalf("connecting to the Kotlin HTTP server: %v", err)
	}
	return session
}

// firstEdictProcessLog returns the named file of the oldest run in root that wrote it: every Edict process logs to
// log/process-log/<run-id>, and run ids are start times, which Glob returns in order.
func firstEdictProcessLog(t *testing.T, root, name string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "log", "process-log", "*", name))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no Edict run in %s wrote %s: %v", root, name, err)
	}
	return matches[0]
}
