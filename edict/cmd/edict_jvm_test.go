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
	"runtime"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func TestEdictDailyRoutineRejectsX86MacOS(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "amd64" {
		t.Skip("requires an x86-64 macOS binary")
	}
	for _, tt := range []struct {
		name string
		args []string
	}{
		{"install", []string{"install"}},
		{"detached server", []string{"mcp", "start"}},
		{"foreground server", []string{"mcp", "start", "--foreground"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			command := NewCommand()
			command.SetArgs(tt.args)
			_, err := executeEdictCommand(context.Background(), command)
			if err == nil || !strings.Contains(err.Error(), "ONNX Runtime no longer ships native libraries") {
				t.Fatalf("expected an unsupported platform error, got %v", err)
			}
			entries, err := os.ReadDir(".")
			if err != nil || len(entries) != 0 {
				t.Fatalf("command should leave the project untouched, got %v, %v", entries, err)
			}
		})
	}
}

func skipUnsupportedEdictRoutine(t *testing.T) {
	t.Helper()
	if err := requireEdictRoutinePlatform(nil, nil); err != nil {
		t.Skip(err)
	}
}

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
	initGitRepository(t, project)
	useFakeCodex(t)
	trusted := trustCodexProject(t, codexHome, project)
	if output := installEdict(t, "--deny", "gold.sarif.json"); !strings.Contains(output, "Installed skills") {
		t.Fatalf("install did not log the installed skills:\n%s", output)
	}
	// Bundled skills may be grouped in folders; each is installed flat under its name.
	for name, root := range bundled {
		err := filepath.WalkDir(
			root, func(path string, entry fs.DirEntry, err error) error {
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
			},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if data, err := os.ReadFile(unrelated); err != nil || string(data) != "keep" {
		t.Fatal("unrelated file changed")
	}
	config := readCodexConfig(t)
	realCodexHome, err := filepath.EvalSymlinks(codexHome)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		strconv.Quote(filepath.Join(realCodexHome, "sessions")) + ` = "deny"`,
		strconv.Quote(filepath.Join(realCodexHome, "log")) + ` = "deny"`,
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

// Install fails, leaving no local config, until Codex trusts the project; it never edits the global Codex config.
func TestEdictInstallRequiresCodexTrust(t *testing.T) {
	skipUnsupportedEdictRoutine(t)
	t.Chdir(t.TempDir())
	project := realWorkingDirectory(t)
	initGitRepository(t, project)
	useFakeCodex(t)
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	globalConfig := filepath.Join(codexHome, "config.toml")
	requireUntrustedInstallFails := func(description string) {
		t.Helper()
		before, _ := os.ReadFile(globalConfig)
		if output, err := runEdictInstall(); err == nil || !strings.Contains(output, "is not trusted") {
			t.Fatalf("install into a project %s: %v\n%s", description, err, output)
		}
		if _, err := os.Stat(filepath.Join(".codex", "config.toml")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed install into a project %s kept the local Codex config: %v", description, err)
		}
		if after, _ := os.ReadFile(globalConfig); !bytes.Equal(before, after) {
			t.Fatalf("failed install into a project %s changed the global Codex config:\n%s", description, after)
		}
	}

	requireUntrustedInstallFails("unknown to Codex")
	untrusted := fmt.Sprintf("[projects.%s]\ntrust_level = \"untrusted\"\n", strconv.Quote(project))
	if err := os.WriteFile(globalConfig, []byte(untrusted), 0o600); err != nil {
		t.Fatal(err)
	}
	requireUntrustedInstallFails("marked untrusted")
	other := fmt.Sprintf("[projects.%s]\ntrust_level = \"trusted\"\n", strconv.Quote(t.TempDir()))
	if err := os.WriteFile(globalConfig, []byte(other), 0o600); err != nil {
		t.Fatal(err)
	}
	requireUntrustedInstallFails("when only another project is trusted")

	trustCodexProject(t, codexHome, project)
	installEdict(t)
}

const fakeCodexEnv = "QODANA_TEST_FAKE_CODEX"

// useFakeCodex points CODEX_BIN at the test binary, which then answers Edict's trust check, `codex mcp list --json`,
// by the rule of Codex: it lists the servers of the project's .codex/config.toml only if CODEX_HOME trusts the
// project. It works without Codex and on every platform.
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
	servers, err := fakeCodexServers()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fake codex: %v\n", err)
		return 1
	}
	answer, err := json.Marshal(servers)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fake codex: %v\n", err)
		return 1
	}
	_, _ = os.Stdout.Write(answer)
	return 0
}

// fakeCodexServers are the MCP servers of ./.codex/config.toml when $CODEX_HOME/config.toml trusts the working
// directory, and none otherwise. It reads only the TOML shapes that trustCodexProject and install write.
func fakeCodexServers() ([]map[string]string, error) {
	servers := []map[string]string{}
	project, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if trusted, err := fakeCodexTrusts(project); err != nil || !trusted {
		return servers, err
	}
	local, err := os.ReadFile(filepath.Join(".codex", "config.toml"))
	if errors.Is(err, os.ErrNotExist) {
		return servers, nil
	}
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(local), "\n") {
		header, _, _ := strings.Cut(strings.TrimSpace(line), "#")
		if name, ok := strings.CutPrefix(strings.TrimSpace(header), "[mcp_servers."); ok && strings.HasSuffix(
			name,
			"]",
		) {
			servers = append(servers, map[string]string{"name": strings.TrimSuffix(name, "]")})
		}
	}
	return servers, nil
}

func fakeCodexTrusts(project string) (bool, error) {
	global, err := os.ReadFile(filepath.Join(os.Getenv("CODEX_HOME"), "config.toml"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		return false, err
	}
	inProject := false
	for _, line := range strings.Split(string(global), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inProject = false
			if quoted, ok := strings.CutPrefix(line, "[projects."); ok {
				path, err := strconv.Unquote(strings.TrimSuffix(quoted, "]"))
				if err != nil {
					return false, err
				}
				resolved, err := filepath.EvalSymlinks(path)
				inProject = err == nil && resolved == project
			}
		} else if inProject && line == `trust_level = "trusted"` {
			return true, nil
		}
	}
	return false, nil
}

// installEdict runs `qodana edict install` in the current directory and returns its console output.
func installEdict(t *testing.T, args ...string) string {
	t.Helper()
	skipUnsupportedEdictRoutine(t)
	output, err := runEdictInstall(args...)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, output)
	}
	return output
}

func runEdictInstall(args ...string) (string, error) {
	command := newEdictInstallCommand()
	// Never nil: cobra would parse the test binary's own os.Args, such as -coverprofile on Windows.
	command.SetArgs(append([]string{}, args...))
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
			// Install ends each line it writes with the "# qodana-edict" marker.
			url, err := strconv.Unquote(strings.TrimSpace(strings.TrimSuffix(value, "# qodana-edict")))
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
	err = filepath.WalkDir(
		bundle, func(path string, entry fs.DirEntry, err error) error {
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
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return skills
}

func TestEdictJVMPropagatesFailure(t *testing.T) {
	skipUnsupportedEdictRoutine(t)
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

// `edict mcp start` serves the Kotlin server over Streamable HTTP at the address install wrote, with the configured state.
func TestEdictMCPStartServesOverHTTP(t *testing.T) {
	project := prepareEdictProject(t, "custom state")
	state := filepath.Join(project, "custom state")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server := startEdictMCP(t, ctx)
	if server.url != edictMCPURL(t) {
		t.Fatalf("server listens at %s, not at the installed address %s", server.url, edictMCPURL(t))
	}
	// Speak Streamable HTTP like an agent host: the SDK client initializes and keeps the session.
	session := connectEdictMCP(t, ctx, server.url)
	registry, err := json.Marshal(callEdictTool(t, ctx, session, "edict_registry", map[string]any{}).StructuredContent)
	if err != nil || !bytes.Contains(registry, []byte(`"edict_manager"`)) || !bytes.Contains(
		registry,
		[]byte(`"edict-distribution"`),
	) {
		t.Fatalf("unexpected Kotlin HTTP registry: %s, %v", registry, err)
	}
	token := createEdictPlan(t, ctx, session, "Check the HTTP server", nil).Token
	runContext := callEdictTool(t, ctx, session, "edict_context", map[string]any{"token": token})
	var paths struct{ ProjectDirectory, StateDirectory, ScratchDirectory string }
	if data, err := json.Marshal(runContext.StructuredContent); err != nil || json.Unmarshal(data, &paths) != nil {
		t.Fatalf("unexpected edict_context result: %s", toolResultText(runContext))
	}
	if paths.ProjectDirectory != project || paths.StateDirectory != state ||
		!strings.HasPrefix(
			paths.ScratchDirectory,
			filepath.Join(project, "log", "agent-work")+string(filepath.Separator),
		) {
		t.Fatalf("unexpected run paths: %+v", paths)
	}
	if info, err := os.Stat(paths.ScratchDirectory); err != nil || !info.IsDir() {
		t.Fatalf("scratch directory was not created: %v", err)
	}
	_ = session.Close()
	server.stop(t, cancel)
	firstEdictProcessLog(t, project, "edict.log")
	firstEdictProcessLog(t, project, "edict-mcp-system.log")
	if _, err := os.Stat(filepath.Join(project, ".edict")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("custom state directory was ignored: %v", err)
	}
}

// The JVM locks its state directory: a second server on it fails, and a fresh one takes it after the first stops.
func TestEdictMCPStartLocksTheStateDirectory(t *testing.T) {
	prepareEdictProject(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server := startEdictMCP(t, ctx)
	if output, err := runEdictMCPStart(ctx); err == nil || !strings.Contains(output, "already owned") {
		t.Fatalf("second JVM did not respect the state lock: %v, %s", err, output)
	}
	server.stop(t, cancel)
	restartContext, restartCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer restartCancel()
	restartEdictMCP(t, restartContext).stop(t, restartCancel)
}

// Another state cannot move to another port: the agent config names the one install wrote.
func TestEdictMCPStartFailsOnABusyPort(t *testing.T) {
	project := prepareEdictProject(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server := startEdictMCP(t, ctx)
	appendEdictStatePath(t, filepath.Join(project, "other state"))
	if output, err := runEdictMCPStart(ctx); err == nil || !strings.Contains(output, "Address already in use") {
		t.Fatalf("second JVM did not fail on the busy port: %v, %s", err, output)
	}
	server.stop(t, cancel)
}

// runEdictMCPStart runs `qodana edict mcp start --foreground` to completion, for a start that is expected to fail.
func runEdictMCPStart(ctx context.Context) (string, error) {
	command := newEdictManagedMCPStartCommand()
	command.SetArgs([]string{"--foreground"})
	command.SetIn(strings.NewReader(""))
	return executeEdictCommand(ctx, command)
}

// callEdictTool calls a tool that must succeed, and fails the test with the server's message otherwise.
func callEdictTool(
	t *testing.T,
	ctx context.Context,
	session *mcp.ClientSession,
	name string,
	arguments map[string]any,
) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s failed: %s", name, toolResultText(result))
	}
	return result
}

// toolResultText is the text a tool returned, such as the message of a failed call.
func toolResultText(result *mcp.CallToolResult) string {
	var text []string
	for _, content := range result.Content {
		if content, ok := content.(*mcp.TextContent); ok {
			text = append(text, content.Text)
		}
	}
	return strings.Join(text, "\n")
}

type edictPlanCreation struct {
	Token string `json:"token"`
	Plan  *struct {
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	} `json:"plan"`
}

// createEdictPlan creates the run's plan, which needs no token, and returns it with the manager token.
func createEdictPlan(
	t *testing.T,
	ctx context.Context,
	session *mcp.ClientSession,
	request string,
	steps []map[string]string,
) edictPlanCreation {
	t.Helper()
	if steps == nil {
		steps = []map[string]string{}
	}
	result := callEdictTool(t, ctx, session, "edict_plan_create", map[string]any{"request": request, "steps": steps})
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var created edictPlanCreation
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatalf("unexpected edict_plan_create result: %v\n%s", err, data)
	}
	if len(created.Token) != 64 || created.Plan == nil {
		t.Fatalf("edict_plan_create returned no manager capability or plan: %s", data)
	}
	return created
}

// prepareEdictProject installs Edict into a fresh project and makes it the current directory. A state directory,
// relative to the project, replaces the default .edict before install checks the state out there.
func prepareEdictProject(t *testing.T, stateDirectory ...string) string {
	t.Helper()
	t.Chdir(t.TempDir())
	project := realWorkingDirectory(t)
	useFakeCodex(t)
	trustCodexProject(t, t.TempDir(), project)
	initGitRepository(t, project)
	// A free port keeps tests independent of a real server on the default port.
	writeEdictPort(t, freePort(t))
	for _, state := range stateDirectory {
		appendEdictStatePath(t, filepath.Join(project, state))
	}
	installEdict(t)
	return project
}

// initGitRepository makes project a git repository, which install requires for the state branch.
func initGitRepository(t *testing.T, project string) {
	t.Helper()
	if output, err := exec.Command("git", "init", "-q", project).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
}

func writeEdictPort(t *testing.T, port int) {
	t.Helper()
	if err := os.WriteFile(
		"qodana.yaml",
		[]byte(fmt.Sprintf("version: \"1.0\"\nedict:\n  mcpPort: %d\n", port)),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}

func moveEdictPort(t *testing.T, port int) {
	t.Helper()
	config, err := os.ReadFile("qodana.yaml")
	if err != nil {
		t.Fatal(err)
	}
	portSetting := regexp.MustCompile(`(?m)^  mcpPort: \d+$`)
	if !portSetting.Match(config) {
		t.Fatal("qodana.yaml has no Edict MCP port to update")
	}
	updated := portSetting.ReplaceAll(config, []byte(fmt.Sprintf("  mcpPort: %d", port)))
	if err := os.WriteFile("qodana.yaml", updated, 0o600); err != nil {
		t.Fatal(err)
	}
}

func appendEdictStatePath(t *testing.T, statePath string) {
	t.Helper()
	file, err := os.OpenFile("qodana.yaml", os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(file, "  statePath: %q\n", statePath)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
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
	for attempt := 1; ; attempt++ {
		server, output, err := tryStartEdictMCP(t, ctx, args...)
		if err == nil {
			return server
		}
		if attempt == 3 || !strings.Contains(output, "Address already in use") {
			t.Fatalf("HTTP server exited before startup (attempt %d): %v\nJVM output:\n%s", attempt, err, output)
		}
		moveEdictPort(t, freePort(t))
		installEdict(t)
	}
}

// restartEdictMCP starts the server on the port a stopped one released, without moving to another port: a port that
// stays bound after stop fails the test.
func restartEdictMCP(t *testing.T, ctx context.Context) edictMCPServer {
	t.Helper()
	server, output, err := tryStartEdictMCP(t, ctx)
	if err != nil {
		t.Fatalf("HTTP server did not restart: %v\nJVM output:\n%s", err, output)
	}
	return server
}

func tryStartEdictMCP(t *testing.T, ctx context.Context, args ...string) (edictMCPServer, string, error) {
	t.Helper()
	output, outputWriter := io.Pipe()
	t.Cleanup(func() { _ = output.Close() })
	command := newEdictManagedMCPStartCommand()
	command.SetArgs(append([]string{"--foreground"}, args...))
	command.SetIn(strings.NewReader(""))
	server := edictMCPServer{done: make(chan error, 1)}
	command.SetOut(outputWriter)
	command.SetErr(outputWriter)
	endpoint := make(chan string, 1)
	scanned := make(chan string, 1)
	go func() {
		var lines []string
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
			if _, url, ok := strings.Cut(line, "edict-mcp listening at "); ok {
				endpoint <- url
			}
		}
		scanned <- strings.Join(lines, "\n")
	}()
	go func() {
		err := command.ExecuteContext(ctx)
		_ = outputWriter.Close()
		server.done <- err
	}()
	select {
	case server.url = <-endpoint:
		return server, "", nil
	case err := <-server.done:
		if err == nil {
			err = errors.New("process exited before listening endpoint")
		}
		return server, <-scanned, err
	case <-ctx.Done():
		err := <-server.done
		return server, <-scanned, fmt.Errorf("HTTP server startup: %w (process error: %v)", ctx.Err(), err)
	}
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

func TestEdictStatePushBeforeInstallLeavesTheStateDirectoryToInstall(t *testing.T) {
	t.Chdir(t.TempDir())
	project := realWorkingDirectory(t)
	useFakeCodex(t)
	trustCodexProject(t, t.TempDir(), project)
	initGitRepository(t, project)
	writeEdictPort(t, freePort(t))
	push := newEdictStateCommand()
	push.SetArgs([]string{"push"})
	push.SetIn(strings.NewReader(""))
	if output, err := executeEdictCommand(context.Background(), push); err == nil || !strings.Contains(
		output,
		"is not a worktree of edict/state",
	) {
		t.Fatalf("state push before install: %v\n%s", err, output)
	}

	output := installEdict(t)

	if !strings.Contains(output, "Started edict/state at "+filepath.Join(project, ".edict")) {
		t.Fatalf("install after a failed state push did not start the state branch:\n%s", output)
	}
}

// gitHubFeedbackConfig writes, outside the project, the Qodana configuration of a project whose reviews are on GitHub,
// which feedback takes the CI of its run from.
func gitHubFeedbackConfig(t *testing.T) string {
	t.Helper()
	config := filepath.Join(t.TempDir(), "qodana.yaml")
	content := "version: \"1.0\"\nedict:\n  ci:\n    url: https://github.com/owner/project\n"
	if err := os.WriteFile(config, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return config
}

func TestEdictFeedbackPrepareForNoPullRequestCopiesNothingAndLogsOutsideTheProject(t *testing.T) {
	t.Chdir(t.TempDir())
	project := realWorkingDirectory(t)
	initGitRepository(t, project)
	event := filepath.Join(t.TempDir(), "event.json")
	if err := os.WriteFile(event, []byte(`{"ref": "refs/heads/main"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_EVENT_PATH", event)
	prepare := newEdictFeedbackCommand()
	output := filepath.Join(t.TempDir(), "inspections")
	prepare.SetArgs([]string{"prepare", "--state-dir", t.TempDir(), "--output-dir", output, "--config", gitHubFeedbackConfig(t)})
	prepare.SetIn(strings.NewReader(""))
	var stdout, stderr bytes.Buffer
	prepare.SetOut(&stdout)
	prepare.SetErr(&stderr)

	if err := prepare.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("feedback prepare: %v\n%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "The workflow runs for no pull request") {
		t.Fatalf("feedback prepare logged:\n%s\n%s", stdout.String(), stderr.String())
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("feedback prepare made the output directory: %v", err)
	}
	if entries, err := os.ReadDir(project); err != nil || len(entries) != 1 || entries[0].Name() != ".git" {
		t.Fatalf("feedback prepare left files in the project: %v %v", entries, err)
	}
}

func TestEdictFeedbackPostForNoPullRequestPostsNothingAndLogsOutsideTheProject(t *testing.T) {
	t.Chdir(t.TempDir())
	project := realWorkingDirectory(t)
	initGitRepository(t, project)
	event := filepath.Join(t.TempDir(), "event.json")
	if err := os.WriteFile(event, []byte(`{"ref": "refs/heads/main"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_EVENT_PATH", event)
	// Nothing reaches the API: the run is for no pull request.
	t.Setenv("GITHUB_API_URL", "https://api.github.invalid")
	t.Setenv("GITHUB_TOKEN", "unused")
	post := newEdictFeedbackCommand()
	post.SetArgs(
		[]string{
			"post",
			"--state-dir",
			t.TempDir(),
			"--sarif",
			filepath.Join(t.TempDir(), "qodana.sarif.json"),
			"--config",
			gitHubFeedbackConfig(t),
		},
	)
	post.SetIn(strings.NewReader(""))
	var stdout, stderr bytes.Buffer
	post.SetOut(&stdout)
	post.SetErr(&stderr)

	if err := post.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("feedback post: %v\n%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "The workflow runs for no pull request") {
		t.Fatalf("feedback post logged:\n%s\n%s", stdout.String(), stderr.String())
	}
	if entries, err := os.ReadDir(project); err != nil || len(entries) != 1 || entries[0].Name() != ".git" {
		t.Fatalf("feedback post left files in the project: %v %v", entries, err)
	}
}
