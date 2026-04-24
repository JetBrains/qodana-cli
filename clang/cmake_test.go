package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/testutil/mockexe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCompilerIncludeResponse is a fake compiler stderr output that contains
// system include directories in the format clang/gcc uses for -v output.
const mockCompilerIncludeResponse = `ignoring nonexistent directory "/usr/local/include"
#include "..." search starts here:
#include <...> search starts here:
 /usr/include
 /usr/lib/clang/include
End of search list.
`

func createMockCompiler(t *testing.T, destPath string, handler func(ctx *mockexe.CallContext) int) string {
	t.Helper()
	return mockexe.CreateMockExe(t, destPath, handler)
}

// TestAskCompiler tests =====

func TestAskCompiler(t *testing.T) {
	t.Run("splits headerType into separate args", func(t *testing.T) {
		tmpDir := t.TempDir()
		var capturedArgs []string
		mockCompiler := createMockCompiler(t, filepath.Join(tmpDir, "compiler"), func(ctx *mockexe.CallContext) int {
			capturedArgs = ctx.Argv
			if _, err := fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse); err != nil {
				ctx.T.Errorf("failed to write to stderr: %v", err)
			}
			return 0
		})

		_, err := askCompiler(mockCompiler, []string{"-E", "-Wp,-v", "-xc++", "/dev/null"})
		require.NoError(t, err)

		// With the current bug, all flags are passed as one argument.
		// After the fix, they should be separate.
		assert.Contains(t, capturedArgs, "-E", "expected -E as separate arg")
		assert.Contains(t, capturedArgs, "-Wp,-v", "expected -Wp,-v as separate arg")
		assert.Contains(t, capturedArgs, "-xc++", "expected -xc++ as separate arg")
		assert.Contains(t, capturedArgs, "/dev/null", "expected /dev/null as separate arg")
	})

	t.Run("parses include directories from stderr", func(t *testing.T) {
		tmpDir := t.TempDir()
		mockCompiler := createMockCompiler(t, filepath.Join(tmpDir, "compiler"), func(ctx *mockexe.CallContext) int {
			if _, err := fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse); err != nil {
				ctx.T.Errorf("failed to write to stderr: %v", err)
			}
			return 0
		})

		headers, err := askCompiler(mockCompiler, []string{"-E", "-Wp,-v", "-xc++", "/dev/null"})
		require.NoError(t, err)
		assert.Contains(t, headers, "--extra-arg=-isystem/usr/include")
		assert.Contains(t, headers, "--extra-arg=-isystem/usr/lib/clang/include")
	})

	t.Run("returns empty headers when no include markers in stderr", func(t *testing.T) {
		tmpDir := t.TempDir()
		mockCompiler := createMockCompiler(t, filepath.Join(tmpDir, "compiler"), func(ctx *mockexe.CallContext) int {
			if _, err := fmt.Fprint(ctx.Stderr, "some unrelated compiler output\n"); err != nil {
				ctx.T.Errorf("failed to write to stderr: %v", err)
			}
			return 0
		})

		headers, err := askCompiler(mockCompiler, []string{"-E", "-Wp,-v", "-xc++", "/dev/null"})
		require.NoError(t, err)
		assert.Empty(t, headers)
	})

	t.Run("sets LC_ALL=C to prevent locale-dependent output", func(t *testing.T) {
		tmpDir := t.TempDir()
		var capturedEnv map[string]string
		mockCompiler := createMockCompiler(t, filepath.Join(tmpDir, "compiler"), func(ctx *mockexe.CallContext) int {
			capturedEnv = ctx.Env
			if _, err := fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse); err != nil {
				ctx.T.Errorf("failed to write to stderr: %v", err)
			}
			return 0
		})

		_, err := askCompiler(mockCompiler, []string{"-E", "-Wp,-v", "-xc++", "/dev/null"})
		require.NoError(t, err)
		assert.Equal(t, "C", capturedEnv["LC_ALL"], "LC_ALL must be set to C to ensure English output from GCC")
	})

	t.Run("returns error on non-zero exit", func(t *testing.T) {
		tmpDir := t.TempDir()
		const stderrMarker = "fatal error: unable to locate system headers"
		mockCompiler := createMockCompiler(t, filepath.Join(tmpDir, "compiler"), func(ctx *mockexe.CallContext) int {
			if _, err := fmt.Fprintln(ctx.Stderr, stderrMarker); err != nil {
				ctx.T.Errorf("failed to write to stderr: %v", err)
			}
			return 1
		})

		_, err := askCompiler(mockCompiler, []string{"-E", "-Wp,-v", "-xc++", "/dev/null"})
		assert.ErrorContains(t, err, "exited with code 1")
		assert.ErrorContains(t, err, stderrMarker)
	})
}

// TestGetFilesAndCompilers tests =====

// toForwardSlash converts backslashes to forward slashes.
// CMake always uses forward slashes in compile_commands.json, even on Windows.
func toForwardSlash(s string) string {
	return strings.ReplaceAll(s, `\`, `/`)
}

func writeCompileCommands(t *testing.T, dir string, commands []Command) string {
	t.Helper()
	data, err := json.Marshal(commands)
	require.NoError(t, err)
	path := filepath.Join(dir, "compile_commands.json")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

func TestGetFilesAndCompilers(t *testing.T) {
	t.Run("extracts quoted compiler path with spaces", func(t *testing.T) {
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "My Tools")
		var invoked atomic.Bool
		mockCompiler := createMockCompiler(t, filepath.Join(toolDir, "gcc"), func(ctx *mockexe.CallContext) int {
			invoked.Store(true)
			if _, err := fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse); err != nil {
				ctx.T.Errorf("failed to write to stderr: %v", err)
			}
			return 0
		})

		ccPath := writeCompileCommands(t, tmpDir, []Command{
			{
				Directory: tmpDir,
				Command:   fmt.Sprintf(`"%s" -c file.cpp`, toForwardSlash(mockCompiler)),
				File:      "file.cpp",
			},
		})

		result, err := getFilesAndCompilers(ccPath)
		require.NoError(t, err)
		assert.True(t, invoked.Load(), "mock compiler was not invoked")
		require.Len(t, result, 1)
		assert.Equal(t, "file.cpp", result[0].File)
		assert.NotEmpty(t, result[0].Headers, "expected non-empty headers")
	})

	t.Run("extracts simple compiler path", func(t *testing.T) {
		tmpDir := t.TempDir()
		var invoked atomic.Bool
		mockCompiler := createMockCompiler(t, filepath.Join(tmpDir, "gcc"), func(ctx *mockexe.CallContext) int {
			invoked.Store(true)
			if _, err := fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse); err != nil {
				ctx.T.Errorf("failed to write to stderr: %v", err)
			}
			return 0
		})

		ccPath := writeCompileCommands(t, tmpDir, []Command{
			{
				Directory: tmpDir,
				Command:   toForwardSlash(mockCompiler) + " -c file.cpp",
				File:      "file.cpp",
			},
		})

		result, err := getFilesAndCompilers(ccPath)
		require.NoError(t, err)
		assert.True(t, invoked.Load(), "mock compiler was not invoked")
		require.Len(t, result, 1)
		assert.NotEmpty(t, result[0].Headers)
	})

	t.Run("uses Arguments when Command is empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		var invoked atomic.Bool
		mockCompiler := createMockCompiler(t, filepath.Join(tmpDir, "gcc"), func(ctx *mockexe.CallContext) int {
			invoked.Store(true)
			if _, err := fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse); err != nil {
				ctx.T.Errorf("failed to write to stderr: %v", err)
			}
			return 0
		})

		ccPath := writeCompileCommands(t, tmpDir, []Command{
			{
				Directory: tmpDir,
				Command:   "",
				Arguments: []string{mockCompiler, "-c", "file.cpp"},
				File:      "file.cpp",
			},
		})

		result, err := getFilesAndCompilers(ccPath)
		require.NoError(t, err)
		assert.True(t, invoked.Load(), "mock compiler was not invoked")
		require.Len(t, result, 1)
	})

	t.Run("skips entry with malformed Command", func(t *testing.T) {
		tmpDir := t.TempDir()

		ccPath := writeCompileCommands(t, tmpDir, []Command{
			{
				Directory: tmpDir,
				Command:   `"/unclosed -c file.cpp`,
				File:      "file.cpp",
			},
		})

		result, err := getFilesAndCompilers(ccPath)
		require.NoError(t, err)
		assert.Empty(t, result, "malformed entry should be skipped")
	})

	t.Run("skips entry with whitespace-only Command and no Arguments", func(t *testing.T) {
		tmpDir := t.TempDir()

		ccPath := writeCompileCommands(t, tmpDir, []Command{
			{Directory: tmpDir, Command: "   ", File: "file.cpp"},
		})

		result, err := getFilesAndCompilers(ccPath)
		require.NoError(t, err)
		assert.Empty(t, result, "whitespace-only command with no arguments should be skipped")
	})

	t.Run("caches headers per compiler and extension", func(t *testing.T) {
		tmpDir := t.TempDir()
		var callCount atomic.Int32
		mockCompiler := createMockCompiler(t, filepath.Join(tmpDir, "gcc"), func(ctx *mockexe.CallContext) int {
			callCount.Add(1)
			if _, err := fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse); err != nil {
				ctx.T.Errorf("failed to write to stderr: %v", err)
			}
			return 0
		})

		ccPath := writeCompileCommands(t, tmpDir, []Command{
			{
				Directory: tmpDir,
				Command:   toForwardSlash(mockCompiler) + " -c a.cpp",
				File:      "a.cpp",
			},
			{
				Directory: tmpDir,
				Command:   toForwardSlash(mockCompiler) + " -c b.cpp",
				File:      "b.cpp",
			},
		})

		result, err := getFilesAndCompilers(ccPath)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, int32(1), callCount.Load(), "compiler should be called only once for same extension")
	})

	t.Run("separate cache for C vs C++", func(t *testing.T) {
		tmpDir := t.TempDir()
		var callCount atomic.Int32
		var capturedArgs [][]string
		mockCompiler := createMockCompiler(t, filepath.Join(tmpDir, "gcc"), func(ctx *mockexe.CallContext) int {
			callCount.Add(1)
			capturedArgs = append(capturedArgs, ctx.Argv)
			if _, err := fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse); err != nil {
				ctx.T.Errorf("failed to write to stderr: %v", err)
			}
			return 0
		})

		ccPath := writeCompileCommands(t, tmpDir, []Command{
			{
				Directory: tmpDir,
				Command:   toForwardSlash(mockCompiler) + " -c file.c",
				File:      "file.c",
			},
			{
				Directory: tmpDir,
				Command:   toForwardSlash(mockCompiler) + " -c file.cpp",
				File:      "file.cpp",
			},
		})

		result, err := getFilesAndCompilers(ccPath)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, int32(2), callCount.Load(), "compiler should be called twice for different extensions")
	})
}

// TestCompilerCacheKey pins that distinct (compiler, headerType) pairs
// produce distinct keys. The helper must use an unambiguous delimiter
// (null byte), because space can appear in both compiler paths and header
// type tokens.
func TestCompilerCacheKey(t *testing.T) {
	t.Run("same inputs produce same key", func(t *testing.T) {
		a := compilerCacheKey("cc", []string{"-E", "-xc", "/dev/null"})
		b := compilerCacheKey("cc", []string{"-E", "-xc", "/dev/null"})
		assert.Equal(t, a, b)
	})

	t.Run("distinguishes compiler trailing space from headerType leading element", func(t *testing.T) {
		a := compilerCacheKey("a ", []string{"b"})
		b := compilerCacheKey("a", []string{" b"})
		assert.NotEqual(t, a, b,
			"cache key must distinguish (%q, %v) from (%q, %v)", "a ", []string{"b"}, "a", []string{" b"})
	})

	t.Run("distinguishes non-empty headerType from suffixed compiler", func(t *testing.T) {
		a := compilerCacheKey("x", []string{"y"})
		b := compilerCacheKey("xy", nil)
		assert.NotEqual(t, a, b,
			"cache key must distinguish (%q, %v) from (%q, %v)", "x", []string{"y"}, "xy", nil)
	})

	t.Run("nil headerType is distinct from any non-empty headerType", func(t *testing.T) {
		nilKey := compilerCacheKey("x", nil)
		assert.NotEqual(t, nilKey, compilerCacheKey("x", []string{""}),
			"nil headerType must not collide with []string{\"\"}")
		assert.NotEqual(t, nilKey, compilerCacheKey("x", []string{"y"}),
			"nil headerType must not collide with non-empty headerType")
	})
}

// TestPickCompiler pins pickCompiler's contract for every branch: valid
// shell-parseable Command, empty Command with Arguments fallback, empty
// Command and no Arguments (skip), shlex-malformed Command with and
// without Arguments fallback, whitespace-only Command, and quoted
// Windows path with spaces.
func TestPickCompiler(t *testing.T) {
	cases := []struct {
		name   string
		cmd    Command
		want   string
		wantOK bool
	}{
		{
			name:   "simple_command",
			cmd:    Command{Command: "gcc -c foo.c", File: "foo.c"},
			want:   "gcc",
			wantOK: true,
		},
		{
			name:   "command_with_leading_whitespace",
			cmd:    Command{Command: "   gcc -c foo.c", File: "foo.c"},
			want:   "gcc",
			wantOK: true,
		},
		{
			name: "quoted_windows_path",
			cmd: Command{
				Command: `"C:\Program Files\LLVM\bin\clang.exe" -c src\main.c`,
				File:    "src/main.c",
			},
			want:   `C:\Program Files\LLVM\bin\clang.exe`,
			wantOK: true,
		},
		{
			name: "empty_command_with_arguments_fallback",
			cmd: Command{
				Command:   "",
				Arguments: []string{"clang", "-c", "foo.c"},
				File:      "foo.c",
			},
			want:   "clang",
			wantOK: true,
		},
		{
			name:   "empty_command_no_arguments_skipped",
			cmd:    Command{Command: "", File: "foo.c"},
			want:   "",
			wantOK: false,
		},
		{
			name: "malformed_with_arguments_falls_back",
			cmd: Command{
				Command:   `"unterminated`,
				Arguments: []string{"clang", "-c", "foo.c"},
				File:      "foo.c",
			},
			want:   "clang",
			wantOK: true,
		},
		{
			name:   "malformed_no_arguments_skipped",
			cmd:    Command{Command: `"unterminated`, File: "foo.c"},
			want:   "",
			wantOK: false,
		},
		{
			name:   "whitespace_only_command_no_arguments_skipped",
			cmd:    Command{Command: "   \t  ", File: "foo.c"},
			want:   "",
			wantOK: false,
		},
		{
			name: "whitespace_only_command_with_arguments_fallback",
			cmd: Command{
				Command:   "   \t  ",
				Arguments: []string{"gcc"},
				File:      "foo.c",
			},
			want:   "gcc",
			wantOK: true,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := pickCompiler(tt.cmd)
			assert.Equal(t, tt.wantOK, ok, "ok mismatch")
			assert.Equal(t, tt.want, got, "compiler mismatch")
		})
	}
}
