package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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

// assertArgsSubsequence checks that `expected` tokens appear in `actual`
// as a subsequence (same relative order, but other tokens may appear
// between them). This is intentionally not a strict-ordering check: the
// canonical argv has tokens (e.g. sarif path, include dirs, input file)
// that are not worth listing in every table case.
func assertArgsSubsequence(t *testing.T, actual, expected []string) {
	t.Helper()
	i := 0
	for _, got := range actual {
		if i < len(expected) && got == expected[i] {
			i++
		}
	}
	if i < len(expected) {
		t.Errorf("expected subsequence %v, missing %q (or out of order) in %v",
			expected, expected[i], actual)
	}
}

func TestPrepareClangArgs(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		want        []string
		expectError bool
	}{
		{
			name: "empty",
			raw:  "",
			want: nil,
		},
		{
			name: "whitespace only",
			raw:  "  ",
			want: nil,
		},
		{
			name: "simple flag gets -- prepended",
			raw:  "-Wall",
			want: []string{"--", "-Wall"},
		},
		{
			name: "multiple flags gets -- prepended once",
			raw:  "-Wall -Werror",
			want: []string{"--", "-Wall", "-Werror"},
		},
		{
			name: "double-quoted path with spaces",
			raw:  `-I"/path/with spaces" -Wall`,
			want: []string{"--", "-I/path/with spaces", "-Wall"},
		},
		{
			name: "single-quoted path with spaces",
			raw:  `-I'/path with spaces' -Wall`,
			want: []string{"--", "-I/path with spaces", "-Wall"},
		},
		{
			name: "backslash-escaped spaces",
			raw:  `-I/path/with\ spaces -Wall`,
			want: []string{"--", "-I/path/with spaces", "-Wall"},
		},
		// POSIX rule: inside "...", \ is only an escape before $ ` " \ or
		// newline. `\P`, `\q` etc. are preserved as-is, so Windows paths
		// quoted with single backslashes round-trip cleanly. Outside quotes,
		// `\X` collapses to `X` (POSIX backslash escapes any single char).
		{
			name: "Windows path, quoted, single backslashes (preserved)",
			raw:  `-I"C:\Projects\qodana-cli" -Wall`,
			want: []string{"--", `-IC:\Projects\qodana-cli`, "-Wall"},
		},
		{
			name: "Windows path, quoted, doubled backslashes (one level consumed)",
			raw:  `-I"C:\\Projects\\qodana-cli" -Wall`,
			want: []string{"--", `-IC:\Projects\qodana-cli`, "-Wall"},
		},
		{
			name: "Windows path, forward slashes, unquoted",
			raw:  `-IC:/Projects/qodana-cli -Wall`,
			want: []string{"--", "-IC:/Projects/qodana-cli", "-Wall"},
		},
		{
			// Unquoted `\X` -> `X`: POSIX-correct but rarely what a Windows
			// user wants. Quote the path.
			name: "Windows path, unquoted, single backslashes (consumed)",
			raw:  `-IC:\Projects\qodana-cli -Wall`,
			want: []string{"--", "-IC:Projectsqodana-cli", "-Wall"},
		},
		{
			// User-supplied `--` => no prepend. Tokens before `--` reach
			// clang-tidy's option parser; tokens after `--` reach the compiler.
			name: "user-supplied -- separator",
			raw:  "--config-file=/tmp/x.tidy -- -Wno-foo",
			want: []string{"--config-file=/tmp/x.tidy", "--", "-Wno-foo"},
		},
		{
			// Bare --config-file= with no `--` still goes to the compiler.
			// To reach clang-tidy, users must include `--` themselves.
			name: "bare --config-file= without -- is treated as compiler arg",
			raw:  "--config-file=/tmp/x.tidy",
			want: []string{"--", "--config-file=/tmp/x.tidy"},
		},
		{
			// `--` alone: contains the separator, no prepend. The empty
			// "before" half just means clang-tidy gets no extra options;
			// the empty "after" half means the compiler gets no extras.
			name: "-- alone",
			raw:  "--",
			want: []string{"--"},
		},
		{
			// Trailing `--` after compiler-style args: caller explicitly
			// terminated the splice. No prepend; the empty after-half is
			// intentional.
			name: "trailing --",
			raw:  "-Wall --",
			want: []string{"-Wall", "--"},
		},
		{
			name:        "unterminated double quote",
			raw:         `foo "`,
			expectError: true,
		},
		{
			name:        "unterminated single quote",
			raw:         `foo '`,
			expectError: true,
		},
		{
			name:        "trailing backslash",
			raw:         `-Wall \`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prepareClangArgs(tt.raw)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid --clang-args",
					"error should identify the offending flag")
				assert.Contains(t, err.Error(), strconv.Quote(tt.raw),
					"error should include the raw value (%%q-quoted)")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRunAnalysis_RejectsMalformedClangArgs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(clangTidySearchRootEnv, tmpDir)

	var invoked atomic.Bool
	fakeClangTidy := mockexe.CreateMockExe(t, filepath.Join(tmpDir, "tools", "clang-tidy"), func(ctx *mockexe.CallContext) int {
		invoked.Store(true)
		return 0
	})

	// Intentionally do NOT create compile_commands.json. RunAnalysis must
	// fail at the prepareClangArgs step before getFilesAndCompilers tries
	// to read it.
	ctx := thirdpartyscan.ContextBuilder{
		ProjectDir:           tmpDir,
		ClangCompileCommands: filepath.Join(tmpDir, "compile_commands.json"),
		ClangArgs:            `foo "`, // unterminated double quote
		MountInfo: thirdpartyscan.MountInfo{
			CustomTools: map[string]string{
				thirdpartyscan.Clang: fakeClangTidy,
			},
		},
	}.Build()

	err := ClangLinter{}.RunAnalysis(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --clang-args",
		"error should identify the offending flag")
	assert.Contains(t, err.Error(), strconv.Quote(`foo "`),
		"error should include the raw value (%q-quoted)")
	assert.False(t, invoked.Load(), "clang-tidy must not be invoked when --clang-args is malformed")
}

// TestLinterRun exercises the clang linter end-to-end against fixtures
// under testdata/. Subtests share a single qodana-jbr cache (downloaded
// once per process via the sync.Once in internal/tooling) by reusing the
// same cacheDir across runs.
func TestLinterRun(t *testing.T) {
	needs.Need(t, needs.ClangDeps)

	log.SetLevel(log.DebugLevel)

	// One shared cacheDir so qodana-jbr is downloaded/extracted only once.
	// Each subtest still gets its own projectDir + outputDir via t.TempDir().
	sharedCacheDir := filepath.Join(t.TempDir(), "linter-cache")

	linterInfo := thirdpartyscan.LinterInfo{
		ProductCode:           product.ClangLinter.ProductCode,
		LinterName:            product.ClangLinter.Name,
		LinterPresentableName: product.ClangLinter.PresentableName,
		LinterVersion:         version,
		IsEap:                 true,
	}

	t.Run("baseline", func(t *testing.T) {
		projectDir := t.TempDir()
		t.Setenv(clangTidySearchRootEnv, projectDir)

		err := os.CopyFS(projectDir, os.DirFS("testdata/TestLinterRun"))
		require.NoError(t, err)

		outputDir := filepath.Join(projectDir, ".linter-output")

		command := platform.NewThirdPartyScanCommand(ClangLinter{}, linterInfo)
		command.SetArgs(
			[]string{
				"-i", projectDir,
				"-o", outputDir,
				"--cache-dir", sharedCacheDir,
			},
		)
		require.NoError(t, command.Execute())

		r, err := platform.ReadReport(filepath.Join(outputDir, "qodana.sarif.json"))
		require.NoError(t, err, "reading SARIF report")
		require.Len(t, r.Runs, 1, "expected exactly one SARIF run")
		require.NotEmpty(t, r.Runs[0].Results, "No results found in SARIF file")

		fmt.Println("Found issues: ", len(r.Runs[0].Results))

		reportDir := filepath.Join(outputDir, "report")
		resultAllProblems, err := os.ReadFile(filepath.Join(platform.ReportResultsPath(reportDir), "result-allProblems.json"))
		require.NoError(t, err, "reading all problems file")
		assert.NotContains(t, string(resultAllProblems), `"listProblem":[]`,
			"All problems file is empty")
	})

	// Exercise the documented --clang-args escape hatch: when the user
	// includes "--" in --clang-args, tokens before it are forwarded to
	// clang-tidy's own option parser (not to the compiler). We point
	// clang-tidy at a custom config file via --config-file= and assert
	// the override took effect — its Checks: setting controls which rules
	// fire.
	t.Run("clang-args config-file override", func(t *testing.T) {
		projectDir := t.TempDir()
		t.Setenv(clangTidySearchRootEnv, projectDir)

		err := os.CopyFS(projectDir, os.DirFS("testdata/TestLinterRun_ClangArgs"))
		require.NoError(t, err)

		// Override _clang-tidy lives OUTSIDE projectDir so qodana's parent
		// walk (capped at projectDir by clangTidySearchRootEnv) cannot find
		// it. The user's --config-file= is the only way it reaches
		// clang-tidy.
		overrideDir := t.TempDir()
		overridePath := filepath.Join(overrideDir, "_clang-tidy")
		err = os.WriteFile(overridePath,
			[]byte("Checks: '-*,readability-magic-numbers'\n"), 0o644)
		require.NoError(t, err)

		outputDir := filepath.Join(projectDir, ".linter-output")

		command := platform.NewThirdPartyScanCommand(ClangLinter{}, linterInfo)
		command.SetArgs(
			[]string{
				"-i", projectDir,
				"-o", outputDir,
				"--cache-dir", sharedCacheDir,
				// The trailing "--" makes everything before it reach
				// clang-tidy's option parser. -Wno-unused after "--" is a
				// (no-op) compiler arg.
				"--clang-args", fmt.Sprintf("--config-file=%s -- -Wno-unused", overridePath),
			},
		)
		require.NoError(t, command.Execute())

		r, err := platform.ReadReport(filepath.Join(outputDir, "qodana.sarif.json"))
		require.NoError(t, err, "reading SARIF report")
		require.Len(t, r.Runs, 1, "expected exactly one SARIF run")

		// The override sets Checks: '-*,readability-magic-numbers'. Because
		// the fixture's .clang-tidy makes processConfig pass no --checks=
		// of its own, the override fully controls which checks run. The
		// only enabled check is readability-magic-numbers, and main.cpp's
		// `int x = 42;` triggers it. Default-checks runs disable
		// readability-magic-numbers, so its presence in the SARIF proves
		// the override reached clang-tidy.
		var ruleIDs []string
		foundMagicNumbers := false
		for _, res := range r.Runs[0].Results {
			ruleIDs = append(ruleIDs, res.RuleId)
			if res.RuleId == "readability-magic-numbers" {
				foundMagicNumbers = true
			}
		}
		assert.True(t, foundMagicNumbers,
			"expected readability-magic-numbers in SARIF results; got rule IDs: %v", ruleIDs)
	})
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
		"",
		nil,
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
		name           string
		checks         string
		extraClangArgs []string
		expectedArgs   []string // if non-nil, assert each token appears in captured args as a subsequence
	}{
		{
			name:           "empty checks and no extra args",
			checks:         "",
			extraClangArgs: nil,
			expectedArgs:   []string{"-p", "--export-sarif", "--quiet"},
		},
		{
			name:           "empty checks with extra args after --",
			checks:         "",
			extraClangArgs: []string{"--", "-Wall"},
			expectedArgs:   []string{"-p", "--export-sarif", "--quiet", "--", "-Wall"},
		},
		{
			name:           "extra args with both clang-tidy and compiler tokens",
			checks:         "--checks=*",
			extraClangArgs: []string{"--config-file=/x", "--", "-Wall"},
			expectedArgs:   []string{"--checks=*", "-p", "--export-sarif", "--quiet", "--config-file=/x", "--", "-Wall"},
		},
		{
			name:           "non-empty checks and extra args",
			checks:         "--checks=*",
			extraClangArgs: []string{"--", "-Wall", "-Werror"},
			expectedArgs:   []string{"--checks=*", "-p", "--export-sarif", "--quiet", "--", "-Wall", "-Werror"},
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
				"",
				tt.extraClangArgs,
				ctx,
				resultsDir,
				stderrCh,
				stdoutCh,
			)

			require.NoError(t, err)
			require.True(t, invoked.Load(), "clang-tidy mock was not invoked")
			require.NotEmpty(t, capturedArgs, "mock captured no args")

			// Skip capturedArgs[0]: per POSIX argv convention (and the
			// mockexe CallContext.Argv contract), argv[0] is the program
			// name exec.Command passed to the process, not an argument the
			// production code constructed.
			for i, arg := range capturedArgs[1:] {
				assert.NotEqual(t, "", arg, "argument at index %d is an empty string", i+1)
			}

			if tt.checks == "" {
				for _, arg := range capturedArgs {
					assert.False(t, strings.HasPrefix(arg, "--checks="),
						"empty checks should not produce a --checks= argument, got %q", arg)
				}
			}

			assertArgsSubsequence(t, capturedArgs, tt.expectedArgs)

			if slices.Contains(tt.extraClangArgs, "--") {
				dashDashIdx := slices.Index(capturedArgs, "--")
				quietIdx := slices.Index(capturedArgs, "--quiet")
				require.GreaterOrEqual(t, dashDashIdx, 0, "captured args must contain --")
				require.GreaterOrEqual(t, quietIdx, 0, "captured args must contain --quiet")
				require.Greater(t, dashDashIdx, quietIdx, "--quiet must appear before --")

				dashDashInExtras := slices.Index(tt.extraClangArgs, "--")
				for _, tok := range tt.extraClangArgs[dashDashInExtras+1:] {
					// Search for the user token at an index strictly after the
					// first `--`. Using slices.Index would return the first
					// occurrence anywhere, which could land before `--` if the
					// token happens to coincide with a production-emitted arg
					// (e.g. `--quiet`).
					found := false
					for k := dashDashIdx + 1; k < len(capturedArgs); k++ {
						if capturedArgs[k] == tok {
							found = true
							break
						}
					}
					assert.True(t, found,
						"user token %q must appear after -- in captured args", tok)
				}
			}
		})
	}
}

func TestRunClangTidy_ConfigFile(t *testing.T) {
	tests := []struct {
		name           string
		configFile     string
		extraClangArgs []string
	}{
		{
			name:           "configFile set, no extra args",
			configFile:     "/tmp/_clang-tidy",
			extraClangArgs: nil,
		},
		{
			name:           "configFile empty",
			configFile:     "",
			extraClangArgs: nil,
		},
		{
			// User's --config-file= must come BEFORE `--` so clang-tidy sees it.
			// Tokens after `--` are forwarded to the compiler, not clang-tidy.
			// The user opted in to this escape hatch by including `--`
			// themselves in --clang-args.
			name:           "configFile set with user override before --",
			configFile:     "/tmp/_clang-tidy",
			extraClangArgs: []string{"--config-file=/user/path", "--"},
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
			require.NoError(t, os.MkdirAll(projectDir, 0o755))
			compileCommands := filepath.Join(projectDir, "compile_commands.json")
			require.NoError(t, os.WriteFile(compileCommands, []byte("[]"), 0o644))
			resultsDir := filepath.Join(tmpDir, "results")
			require.NoError(t, os.MkdirAll(resultsDir, 0o755))

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
				"",
				tt.configFile,
				tt.extraClangArgs,
				ctx,
				resultsDir,
				stderrCh,
				stdoutCh,
			)
			require.NoError(t, err)
			require.True(t, invoked.Load(), "mock was not invoked")

			configFileTokens := []int{}
			for i, arg := range capturedArgs {
				if strings.HasPrefix(arg, "--config-file=") {
					configFileTokens = append(configFileTokens, i)
				}
			}

			userHasConfigFile := slices.ContainsFunc(tt.extraClangArgs, func(s string) bool {
				return strings.HasPrefix(s, "--config-file=")
			})
			dashDashIdx := slices.Index(tt.extraClangArgs, "--")

			switch {
			case tt.configFile == "" && !userHasConfigFile:
				assert.Empty(t, configFileTokens,
					"no --config-file= token expected when configFile is empty and extras have none")

			case tt.configFile != "" && !userHasConfigFile:
				require.Len(t, configFileTokens, 1)
				assert.Equal(t, "--config-file="+tt.configFile, capturedArgs[configFileTokens[0]])

			case tt.configFile != "" && userHasConfigFile:
				require.Len(t, configFileTokens, 2,
					"both qodana's and user's --config-file= should be present")
				// Qodana's appears first, user's wins via last-occurrence semantics.
				assert.Equal(t, "--config-file="+tt.configFile, capturedArgs[configFileTokens[0]])
				assert.Equal(t, "--config-file=/user/path", capturedArgs[configFileTokens[1]])
				assert.Less(t, configFileTokens[0], configFileTokens[1],
					"user's --config-file= must come after qodana's so it wins")
				// Both must appear before `--` so clang-tidy's option parser sees them.
				if dashDashIdx >= 0 {
					captureDashDashIdx := slices.Index(capturedArgs, "--")
					require.GreaterOrEqual(t, captureDashDashIdx, 0)
					assert.Less(t, configFileTokens[1], captureDashDashIdx,
						"user's --config-file= must appear before -- to reach clang-tidy")
				}
			}
		})
	}
}
