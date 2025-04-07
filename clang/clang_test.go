package main

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/v2025/platform"
	log "github.com/sirupsen/logrus"
)

func TestLinterRun(t *testing.T) {
	// skip test on GH, since required jars are not there
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip()
	}
	log.SetLevel(log.DebugLevel)
	projectPath := createNativeProject(t, "cpp-demo")
	defer deferredCleanup(projectPath)
	outputDir := filepath.Join(os.TempDir(), "clang-output")
	defer deferredCleanup(outputDir)
	cacheDir := filepath.Join(os.TempDir(), "clangTmp")
	defer deferredCleanup(cacheDir)

	linterInfo := thirdpartyscan.LinterInfo{
		ProductCode:   productCode,
		LinterName:    linterName,
		LinterVersion: "2023.3",
		IsEap:         true,
	}

	command := platform.NewThirdPartyScanCommand(ClangLinter{}, linterInfo)
	command.SetArgs([]string{"-i", projectPath, "-o", outputDir, "--cache-dir", cacheDir})
	err := command.Execute()
	defer deferredCleanup(cacheDir)
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

	resultAllProblems, err := os.ReadFile(filepath.Join(outputDir, "report", "results", "result-allProblems.json"))
	if err != nil {
		t.Fatal("Error reading all problems file", err)
	}

	allProblems := string(resultAllProblems)
	if strings.Contains(allProblems, `"listProblem":[]`) {
		t.Fatal("All problems file is empty")
	}
}

func deferredCleanup(path string) {
	err := os.RemoveAll(path)
	if err != nil {
		log.Fatal(err)
	}
}

func createNativeProject(t *testing.T, name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	location := filepath.Join(home, ".qodana_scan_", name)
	err = gitClone("https://github.com/hybloid/cpp-demo", location)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func gitClone(repoURL, directory string) error {
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		err = os.RemoveAll(directory)
		if err != nil {
			return err
		}
	}
	cmd := exec.Command("git", "clone", repoURL, directory)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
