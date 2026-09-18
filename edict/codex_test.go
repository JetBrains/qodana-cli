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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrepareCodexHome(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	readOnly := t.TempDir()
	writable := t.TempDir()
	denied := t.TempDir()
	t.Setenv("LITELLM_API_KEY", "configured-for-test")
	home, err := PrepareCodexHome(CodexHomeConfig{
		Directory:          filepath.Join(t.TempDir(), "codex-home"),
		Executable:         executable,
		PermissionsProfile: "test-profile",
		ReadOnlyPaths:      []string{readOnly},
		DeniedPaths:        []string{denied},
		WritableRoots:      []string{writable},
		Network: CodexNetworkPermissions{
			Enabled:                        true,
			Mode:                           "full",
			AllowLocalBinding:              true,
			DangerouslyAllowAllUnixSockets: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	configuration := string(content)
	for _, expected := range []string{
		`model_provider = "litellm"`,
		`default_permissions = "test-profile"`,
		tomlString(executable) + ` = "read"`,
		tomlString(readOnly) + ` = "read"`,
		tomlString(denied) + ` = "deny"`,
		tomlString(writable) + ` = true`,
		`[permissions.test-profile.network]`,
		`dangerously_allow_all_unix_sockets = true`,
	} {
		if !strings.Contains(configuration, expected) {
			t.Errorf("Codex configuration does not contain %q\n%s", expected, configuration)
		}
	}
}

func TestRunCodex(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX Codex stub")
	}
	temporary := t.TempDir()
	executable := filepath.Join(temporary, "codex-stub")
	stub := `#!/bin/sh
test "$CODEX_HOME" = "$EXPECTED_CODEX_HOME" || exit 20
last_message=
prompt=
while test "$#" -gt 0; do
  case "$1" in
    --output-last-message)
      last_message=$2
      shift 2
      ;;
    *)
      prompt=$1
      shift
      ;;
  esac
done
printf '%s\n' '{"type":"turn.completed"}'
printf '%s\n' 'stub stderr' >&2
printf '%s' "$prompt" > "$last_message"
`
	if err := os.WriteFile(executable, []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(temporary, "codex-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXPECTED_CODEX_HOME", home)
	result, err := RunCodex(context.Background(), CodexRunConfig{
		Executable:       executable,
		HomeDirectory:    home,
		WorkingDirectory: temporary,
		OutputDirectory:  filepath.Join(temporary, "traces"),
		Model:            "test-model",
		Prompt:           "test prompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stdout, `{"type":"turn.completed"}`) {
		t.Errorf("unexpected stdout: %s", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "stub stderr") {
		t.Errorf("unexpected stderr: %s", result.Stderr)
	}
	if result.LastMessage != "test prompt" {
		t.Errorf("unexpected last message: %q", result.LastMessage)
	}
}

func TestInstallSkill(t *testing.T) {
	destination := t.TempDir()
	if err := InstallSkill(destination, "edict-signal-history-examples"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "edict-signal-history-examples", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := InstallSkill(destination, "missing-skill"); err == nil {
		t.Fatal("installing an unknown skill succeeded")
	}
}
