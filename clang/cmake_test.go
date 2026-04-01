package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
			_, _ = fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse)
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
			_, _ = fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse)
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
			_, _ = fmt.Fprint(ctx.Stderr, "some unrelated compiler output\n")
			return 0
		})

		headers, err := askCompiler(mockCompiler, []string{"-E", "-Wp,-v", "-xc++", "/dev/null"})
		require.NoError(t, err)
		assert.Empty(t, headers)
	})

	t.Run("returns error on non-zero exit", func(t *testing.T) {
		tmpDir := t.TempDir()
		mockCompiler := createMockCompiler(t, filepath.Join(tmpDir, "compiler"), func(ctx *mockexe.CallContext) int {
			return 1
		})

		_, err := askCompiler(mockCompiler, []string{"-E", "-Wp,-v", "-xc++", "/dev/null"})
		assert.Error(t, err)
	})
}

// TestGetFilesAndCompilers tests =====

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
			_, _ = fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse)
			return 0
		})

		ccPath := writeCompileCommands(t, tmpDir, []Command{
			{
				Directory: tmpDir,
				Command:   fmt.Sprintf(`"%s" -c file.cpp`, mockCompiler),
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
			_, _ = fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse)
			return 0
		})

		ccPath := writeCompileCommands(t, tmpDir, []Command{
			{
				Directory: tmpDir,
				Command:   mockCompiler + " -c file.cpp",
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
			_, _ = fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse)
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

	t.Run("caches headers per compiler and extension", func(t *testing.T) {
		tmpDir := t.TempDir()
		var callCount atomic.Int32
		mockCompiler := createMockCompiler(t, filepath.Join(tmpDir, "gcc"), func(ctx *mockexe.CallContext) int {
			callCount.Add(1)
			_, _ = fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse)
			return 0
		})

		ccPath := writeCompileCommands(t, tmpDir, []Command{
			{
				Directory: tmpDir,
				Command:   mockCompiler + " -c a.cpp",
				File:      "a.cpp",
			},
			{
				Directory: tmpDir,
				Command:   mockCompiler + " -c b.cpp",
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
			_, _ = fmt.Fprint(ctx.Stderr, mockCompilerIncludeResponse)
			return 0
		})

		ccPath := writeCompileCommands(t, tmpDir, []Command{
			{
				Directory: tmpDir,
				Command:   mockCompiler + " -c file.c",
				File:      "file.c",
			},
			{
				Directory: tmpDir,
				Command:   mockCompiler + " -c file.cpp",
				File:      "file.cpp",
			},
		})

		result, err := getFilesAndCompilers(ccPath)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, int32(2), callCount.Load(), "compiler should be called twice for different extensions")
	})
}
