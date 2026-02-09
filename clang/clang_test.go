package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/stretchr/testify/assert"
	log "github.com/sirupsen/logrus"
)

func TestLinterRun(t *testing.T) {
	// skip this test on GitHub due to missing artifacts
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip()
	}

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

	resultAllProblems, err := os.ReadFile(filepath.Join(outputDir, "report", "result-allProblems.json"))
	if err != nil {
		t.Fatal("Error reading all problems file", err)
	}

	allProblems := string(resultAllProblems)
	if strings.Contains(allProblems, `"listProblem":[]`) {
		t.Fatal("All problems file is empty")
	}
}

func TestRunClangTidy_PathWithSpaces(t *testing.T) {
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS == "windows" {
		t.Skip("test uses a shell script as a fake clang-tidy")
	}

	tmpDir := t.TempDir()

	// Place the fake clang-tidy in a directory with spaces
	toolDir := filepath.Join(tmpDir, "My Tools")
	assert.NoError(t, os.MkdirAll(toolDir, 0o755))

	fakeClangTidy := filepath.Join(toolDir, "clang-tidy")
	// Script writes an empty SARIF to the --export-sarif path (arg after --export-sarif flag)
	script := `#!/bin/sh
for arg; do
  prev_was_export=false
done
# Parse --export-sarif argument
i=0
for arg; do
  if [ "$prev" = "--export-sarif" ]; then
    echo '{}' > "$arg"
    exit 0
  fi
  prev="$arg"
done
exit 0
`
	assert.NoError(t, os.WriteFile(fakeClangTidy, []byte(script), 0o755))

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
}
