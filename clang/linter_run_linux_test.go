//go:build linux

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/internal/testutil/needs"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// qodana-clang only runs inside a Linux container, so the tests that invoke the real (embedded)
// clang-tidy live here, compiled only on Linux. needs.ClangDeps still gates the QT_ENABLE_CLANG_DEPS=0
// escape hatch for a Linux dev without the closed-source archive.

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
		IsEap:                 product.ClangLinter.EapOnly,
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
				//
				// --clang-args is split with POSIX shell rules, which would eat the
				// backslashes of a Windows path; clang-tidy accepts forward slashes on
				// every platform, so forward-slash the override path to stay portable.
				"--clang-args", fmt.Sprintf("--config-file=%s -- -Wno-unused", filepath.ToSlash(overridePath)),
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

// TestDefaultChecks_AcceptedByClangTidy is a smoke test that the curated
// defaultChecks string is accepted by the real (embedded) clang-tidy. Runs
// clang-tidy --list-checks --checks=<defaultChecks> and confirms:
//   - a few expected category names appear in stdout,
//   - stderr carries no "unknown check" warnings (typo guard).
func TestDefaultChecks_AcceptedByClangTidy(t *testing.T) {
	needs.Need(t, needs.ClangDeps)

	tmpDir := t.TempDir()
	mountInfo, err := ClangLinter{}.MountTools(tmpDir)
	require.NoError(t, err, "mounting the embedded clang-tidy must succeed when ClangDeps is on")
	bin := mountInfo[thirdpartyscan.Clang]

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, "--list-checks", "--checks="+defaultChecks)
	// Force English output so the typo-guard substring match below is locale-stable.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("clang-tidy rejected defaultChecks: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{"bugprone-", "performance-", "readability-"} {
		assert.Contains(t, out, want, "enabled check list should contain %q", want)
	}
	errOut := stderr.String()
	assert.NotContains(t, errOut, "unknown check",
		"defaultChecks must not reference unknown checks; stderr: %s", errOut)
	assert.NotContains(t, errOut, "does not exist",
		"defaultChecks must not reference non-existent checks; stderr: %s", errOut)
}
