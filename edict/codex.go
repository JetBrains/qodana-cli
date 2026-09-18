/*
 * Copyright 2021-2026 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package edict

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// DefaultCodexModel is used when CODEX_MODEL is not configured.
const DefaultCodexModel = "gpt-5.6-sol"

var codexPermissionsProfilePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// CodexNetworkPermissions controls the network section of an isolated Codex permission profile.
type CodexNetworkPermissions struct {
	Enabled                        bool
	Mode                           string
	AllowLocalBinding              bool
	DangerouslyAllowAllUnixSockets bool
}

// CodexHomeConfig describes an isolated CODEX_HOME and its filesystem permissions.
type CodexHomeConfig struct {
	Directory          string
	Executable         string
	PermissionsProfile string
	ReadOnlyPaths      []string
	DeniedPaths        []string
	WritableRoots      []string
	Network            CodexNetworkPermissions
}

// CodexRunConfig describes one non-interactive Codex invocation and its trace location.
type CodexRunConfig struct {
	Executable       string
	HomeDirectory    string
	WorkingDirectory string
	OutputDirectory  string
	Model            string
	Prompt           string
}

// CodexRunResult contains captured traces and the final assistant message.
type CodexRunResult struct {
	StdoutPath      string
	StderrPath      string
	LastMessagePath string
	Stdout          string
	Stderr          string
	LastMessage     string
}

// CombinedOutput returns stdout followed by stderr for diagnostics and event inspection.
func (result CodexRunResult) CombinedOutput() string {
	return result.Stdout + result.Stderr
}

// ResolveCodexExecutable finds CODEX_BIN, or codex from PATH, and resolves symlinks.
func ResolveCodexExecutable() (string, error) {
	executable := strings.TrimSpace(os.Getenv("CODEX_BIN"))
	if executable == "" {
		executable = "codex"
	}
	resolved, err := exec.LookPath(executable)
	if err != nil {
		return "", fmt.Errorf("find Codex executable: %w", err)
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve Codex executable: %w", err)
	}
	return resolved, nil
}

// CodexModelFromEnvironment returns CODEX_MODEL or DefaultCodexModel.
func CodexModelFromEnvironment() string {
	if model := strings.TrimSpace(os.Getenv("CODEX_MODEL")); model != "" {
		return model
	}
	return DefaultCodexModel
}

// PrepareCodexHome writes a provider-aware config.toml for an isolated Codex invocation.
func PrepareCodexHome(config CodexHomeConfig) (string, error) {
	if strings.TrimSpace(config.Directory) == "" {
		return "", errors.New("Codex home directory is required")
	}
	if strings.TrimSpace(config.Executable) == "" {
		return "", errors.New("Codex executable is required")
	}
	if !codexPermissionsProfilePattern.MatchString(config.PermissionsProfile) {
		return "", fmt.Errorf("invalid Codex permissions profile %q", config.PermissionsProfile)
	}
	home, err := filepath.Abs(config.Directory)
	if err != nil {
		return "", fmt.Errorf("resolve Codex home: %w", err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return "", fmt.Errorf("create Codex home: %w", err)
	}

	readPermissions, err := codexPathPermissions(append([]string{config.Executable}, config.ReadOnlyPaths...), `"read"`)
	if err != nil {
		return "", err
	}
	deniedPermissions, err := codexPathPermissions(config.DeniedPaths, `"deny"`)
	if err != nil {
		return "", err
	}
	writablePermissions, err := codexPathPermissions(config.WritableRoots, "true")
	if err != nil {
		return "", err
	}
	providerConfig := ""
	if strings.TrimSpace(os.Getenv("LITELLM_API_KEY")) != "" {
		providerConfig = `model_provider = "litellm"

[model_providers.litellm]
name = "LiteLLM"
base_url = "https://litellm.labs.jb.gg/openai"
env_key = "LITELLM_API_KEY"
wire_api = "responses"

`
	}
	networkConfig := ""
	if config.Network.Enabled {
		mode := strings.TrimSpace(config.Network.Mode)
		if mode == "" {
			mode = "full"
		}
		networkConfig = fmt.Sprintf(`
[permissions.%s.network]
enabled = true
mode = %s
allow_local_binding = %s
dangerously_allow_all_unix_sockets = %s
`, config.PermissionsProfile, tomlString(mode),
			strconv.FormatBool(config.Network.AllowLocalBinding),
			strconv.FormatBool(config.Network.DangerouslyAllowAllUnixSockets))
	}
	content := fmt.Sprintf(`approval_policy = "never"
default_permissions = %s
model_reasoning_effort = "high"

%s[permissions.%s]
extends = ":read-only"

[permissions.%s.filesystem]
":tmpdir" = "write"
%s%s
[permissions.%s.workspace_roots]
%s
[permissions.%s.filesystem.":workspace_roots"]
"." = "write"
%s`, tomlString(config.PermissionsProfile), providerConfig, config.PermissionsProfile,
		config.PermissionsProfile, deniedPermissions, readPermissions, config.PermissionsProfile,
		writablePermissions, config.PermissionsProfile, networkConfig)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write Codex configuration: %w", err)
	}
	return home, nil
}

// RunCodex runs one non-interactive Codex task and captures its JSONL, stderr, and final message.
func RunCodex(ctx context.Context, config CodexRunConfig) (CodexRunResult, error) {
	var result CodexRunResult
	if ctx == nil {
		return result, errors.New("Codex context is required")
	}
	if strings.TrimSpace(config.Executable) == "" || strings.TrimSpace(config.HomeDirectory) == "" {
		return result, errors.New("Codex executable and home directory are required")
	}
	if strings.TrimSpace(config.WorkingDirectory) == "" || strings.TrimSpace(config.OutputDirectory) == "" {
		return result, errors.New("Codex working and output directories are required")
	}
	if strings.TrimSpace(config.Model) == "" || strings.TrimSpace(config.Prompt) == "" {
		return result, errors.New("Codex model and prompt are required")
	}
	if err := os.MkdirAll(config.OutputDirectory, 0o755); err != nil {
		return result, fmt.Errorf("create Codex output directory: %w", err)
	}
	result.StdoutPath = filepath.Join(config.OutputDirectory, "stdout.jsonl")
	result.StderrPath = filepath.Join(config.OutputDirectory, "stderr.log")
	result.LastMessagePath = filepath.Join(config.OutputDirectory, "codex-last-message.txt")
	if err := os.Remove(result.LastMessagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, fmt.Errorf("clear previous Codex last message: %w", err)
	}
	stdout, err := os.Create(result.StdoutPath)
	if err != nil {
		return result, fmt.Errorf("create Codex stdout trace: %w", err)
	}
	stderr, err := os.Create(result.StderrPath)
	if err != nil {
		_ = stdout.Close()
		return result, fmt.Errorf("create Codex stderr trace: %w", err)
	}

	command := exec.CommandContext(ctx, config.Executable,
		"exec",
		"--dangerously-bypass-hook-trust",
		"--json",
		"--skip-git-repo-check",
		"--model", config.Model,
		"--output-last-message", result.LastMessagePath,
		config.Prompt,
	)
	command.Dir = config.WorkingDirectory
	command.Stdin = strings.NewReader("")
	command.Env = environmentWithOverrides(map[string]string{"CODEX_HOME": config.HomeDirectory})
	command.Stdout = stdout
	command.Stderr = stderr
	runErr := command.Run()
	closeErr := errors.Join(stdout.Close(), stderr.Close())

	result.Stdout, err = readCodexTrace(result.StdoutPath)
	readErr := err
	result.Stderr, err = readCodexTrace(result.StderrPath)
	readErr = errors.Join(readErr, err)
	result.LastMessage, err = readCodexLastMessage(result.LastMessagePath, runErr)
	readErr = errors.Join(readErr, err)
	if combined := errors.Join(runErr, closeErr, readErr); combined != nil {
		return result, fmt.Errorf("run Codex: %w", combined)
	}
	return result, nil
}

func codexPathPermissions(paths []string, value string) (string, error) {
	seen := make(map[string]struct{}, len(paths))
	var lines []string
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve Codex permission path %q: %w", path, err)
		}
		if _, duplicate := seen[absolute]; duplicate {
			continue
		}
		seen[absolute] = struct{}{}
		lines = append(lines, tomlString(absolute)+" = "+value)
	}
	if len(lines) == 0 {
		return "", nil
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func readCodexTrace(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read Codex trace %s: %w", path, err)
	}
	return string(content), nil
}

func readCodexLastMessage(path string, runErr error) (string, error) {
	content, err := os.ReadFile(path)
	if err == nil {
		return string(content), nil
	}
	if runErr != nil && errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return "", fmt.Errorf("read Codex last message %s: %w", path, err)
}

func environmentWithOverrides(overrides map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[key]; !replaced {
			environment = append(environment, entry)
		}
	}
	for key, value := range overrides {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func tomlString(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}
