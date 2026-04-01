package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/internal/testutil/mockexe"
	"github.com/JetBrains/qodana-cli/internal/testutil/needs"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinterRun(t *testing.T) {
	needs.Need(t, needs.ClangDeps)

	log.SetLevel(log.DebugLevel)

	projectDir := t.TempDir()

	err := os.CopyFS(projectDir, os.DirFS("testdata/TestLinterRun"))
	if err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(projectDir, ".linter-output")
	cacheDir := filepath.Join(projectDir, ".linter-cache")

	linterInfo := thirdpartyscan.LinterInfo{
		ProductCode:           product.ClangLinter.ProductCode,
		LinterName:            product.ClangLinter.Name,
		LinterPresentableName: product.ClangLinter.PresentableName,
		LinterVersion:         version,
		IsEap:                 true,
	}

	command := platform.NewThirdPartyScanCommand(ClangLinter{}, linterInfo)
	command.SetArgs(
		[]string{
			"-i", projectDir,
			"-o", outputDir,
			"--cache-dir", cacheDir,
		},
	)
	err = command.Execute()
	if err != nil {
		t.Fatal(err)
	}

	r, err := platform.ReadReport(filepath.Join(outputDir, "qodana.sarif.json"))
	if err != nil {
		t.Fatal("Error reading report", err)
	}

	if len(r.Runs) != 1 {
		t.Fatal("Expected 1 run in SARIF file, but got", len(r.Runs))
	}

	resultsSize := len(r.Runs[0].Results)
	if resultsSize == 0 {
		t.Fatal("No results found in SARIF file")
	}
	fmt.Println("Found issues: ", resultsSize)

	reportDir := filepath.Join(outputDir, "report")
	resultAllProblems, err := os.ReadFile(filepath.Join(platform.ReportResultsPath(reportDir), "result-allProblems.json"))
	if err != nil {
		t.Fatal("Error reading all problems file", err)
	}

	allProblems := string(resultAllProblems)
	if strings.Contains(allProblems, `"listProblem":[]`) {
		t.Fatal("All problems file is empty")
	}
}

func TestRunClangTidy_PathWithSpaces(t *testing.T) {
	tmpDir := t.TempDir()

	// Place the fake clang-tidy in a directory with spaces
	toolDir := filepath.Join(tmpDir, "My Tools")
	var invoked atomic.Bool
	fakeClangTidy := mockexe.CreateMockExe(t, filepath.Join(toolDir, "clang-tidy"), func(ctx *mockexe.CallContext) int {
		invoked.Store(true)
		for i, arg := range ctx.Argv {
			if arg == "--export-sarif" && i+1 < len(ctx.Argv) {
				_ = os.MkdirAll(filepath.Dir(ctx.Argv[i+1]), 0o755)
				_ = os.WriteFile(ctx.Argv[i+1], []byte("{}"), 0o644)
				break
			}
		}
		return 0
	})

	// compile_commands.json in a directory with spaces
	projectDir := filepath.Join(tmpDir, "my project")
	assert.NoError(t, os.MkdirAll(projectDir, 0o755))
	compileCommands := filepath.Join(projectDir, "compile_commands.json")
	assert.NoError(t, os.WriteFile(compileCommands, []byte("[]"), 0o644))

	resultsDir := filepath.Join(tmpDir, "my results")
	assert.NoError(t, os.MkdirAll(resultsDir, 0o755))

	ctx := thirdpartyscan.ContextBuilder{
		ProjectDir:           projectDir,
		ClangCompileCommands: compileCommands,
		MountInfo: thirdpartyscan.MountInfo{
			CustomTools: map[string]string{
				thirdpartyscan.Clang: fakeClangTidy,
			},
		},
	}.Build()

	stderrCh := make(chan string, 1)
	stdoutCh := make(chan string, 1)

	err := runClangTidy(
		0,
		FileWithHeaders{File: filepath.Join(projectDir, "test.c")},
		"-checks=-*",
		ctx,
		resultsDir,
		stderrCh,
		stdoutCh,
	)
	assert.NoError(t, err)
	assert.True(t, invoked.Load(), "clang-tidy mock was not invoked")
}

func TestRunClangTidy_NoEmptyArgs(t *testing.T) {
	tests := []struct {
		name         string
		checks       string
		clangArgs    string
		expectedArgs []string // if non-nil, assert each token appears in captured args
		expectError  bool
	}{
		{
			name:      "empty checks and empty ClangArgs",
			checks:    "",
			clangArgs: "",
		},
		{
			name:      "empty checks with non-empty ClangArgs",
			checks:    "",
			clangArgs: "-- -Wall",
		},
		{
			name:      "non-empty ClangArgs with double spaces",
			checks:    "--checks=*",
			clangArgs: "-- -Wall  -Werror",
		},
		{
			name:      "whitespace-only ClangArgs",
			checks:    "--checks=*",
			clangArgs: "  ",
		},
		{
			name:      "non-empty checks and ClangArgs",
			checks:    "--checks=*",
			clangArgs: "-- -Wall",
		},
		{
			name:         "double-quoted path with spaces",
			checks:       "--checks=*",
			clangArgs:    `-- -I"/path/with spaces" -Wall`,
			expectedArgs: []string{"--", "-I/path/with spaces", "-Wall"},
		},
		{
			name:         "single-quoted path with spaces",
			checks:       "--checks=*",
			clangArgs:    `-- -I'/path with spaces' -Wall`,
			expectedArgs: []string{"--", "-I/path with spaces", "-Wall"},
		},
		{
			name:         "backslash-escaped spaces",
			checks:       "--checks=*",
			clangArgs:    `-- -I/path/with\ spaces -Wall`,
			expectedArgs: []string{"--", "-I/path/with spaces", "-Wall"},
		},
		{
			name:        "unclosed quote error",
			checks:      "--checks=*",
			clangArgs:   `-- -I"/unclosed`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			toolDir := filepath.Join(tmpDir, "tools")
			var invoked atomic.Bool
			var capturedArgs []string
			fakeClangTidy := mockexe.CreateMockExe(t, filepath.Join(toolDir, "clang-tidy"), func(ctx *mockexe.CallContext) int {
				invoked.Store(true)
				capturedArgs = ctx.Argv
				for i, arg := range ctx.Argv {
					if arg == "--export-sarif" && i+1 < len(ctx.Argv) {
						_ = os.MkdirAll(filepath.Dir(ctx.Argv[i+1]), 0o755)
						_ = os.WriteFile(ctx.Argv[i+1], []byte("{}"), 0o644)
						break
					}
				}
				return 0
			})

			projectDir := filepath.Join(tmpDir, "project")
			assert.NoError(t, os.MkdirAll(projectDir, 0o755))
			compileCommands := filepath.Join(projectDir, "compile_commands.json")
			assert.NoError(t, os.WriteFile(compileCommands, []byte("[]"), 0o644))

			resultsDir := filepath.Join(tmpDir, "results")
			assert.NoError(t, os.MkdirAll(resultsDir, 0o755))

			ctx := thirdpartyscan.ContextBuilder{
				ProjectDir:           projectDir,
				ClangCompileCommands: compileCommands,
				ClangArgs:            tt.clangArgs,
				MountInfo: thirdpartyscan.MountInfo{
					CustomTools: map[string]string{
						thirdpartyscan.Clang: fakeClangTidy,
					},
				},
			}.Build()

			stderrCh := make(chan string, 1)
			stdoutCh := make(chan string, 1)

			err := runClangTidy(
				0,
				FileWithHeaders{File: filepath.Join(projectDir, "test.c")},
				tt.checks,
				ctx,
				resultsDir,
				stderrCh,
				stdoutCh,
			)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.True(t, invoked.Load(), "clang-tidy mock was not invoked")

			for i, arg := range capturedArgs[1:] {
				assert.NotEqual(t, "", arg, "argument at index %d is an empty string", i+1)
			}

			for _, expected := range tt.expectedArgs {
				assert.Contains(t, capturedArgs, expected, "expected argument %q not found in args", expected)
			}
		})
	}
}
