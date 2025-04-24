package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"

	"github.com/JetBrains/qodana-cli/v2025/platform"
	log "github.com/sirupsen/logrus"
)

func TestLinterRun(t *testing.T) {
	// skip test on GH, since required jars are not there
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
		ProductCode:   productCode,
		LinterName:    linterName,
		LinterVersion: version,
		IsEap:         true,
	}

	command := platform.NewThirdPartyScanCommand(ClangLinter{}, linterInfo)
	command.SetArgs([]string{"-i", projectDir, "-o", outputDir, "--cache-dir", cacheDir})
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
	} else {
		fmt.Println("Found issues: ", resultsSize)
	}

	resultAllProblems, err := os.ReadFile(filepath.Join(outputDir, "report", "result-allProblems.json"))
	if err != nil {
		t.Fatal("Error reading all problems file", err)
	}

	allProblems := string(resultAllProblems)
	if strings.Contains(allProblems, `"listProblem":[]`) {
		t.Fatal("All problems file is empty")
	}
}
