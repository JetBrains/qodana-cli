/*
 * Copyright 2021-2024 JetBrains s.r.o.
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

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/v2025/platform/product"

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
		ProductCode:           product.DotNetCommunityLinter.ProductCode,
		LinterPresentableName: product.DotNetCommunityLinter.PresentableName,
		LinterName:            product.DotNetCommunityLinter.Name,
		LinterVersion:         version,
		IsEap:                 true,
	}

	command := platform.NewThirdPartyScanCommand(CdnetLinter{}, linterInfo)
	command.SetArgs(
		[]string{
			"-i",
			projectDir,
			"--repository-root",
			projectDir,
			"-o",
			outputDir,
			"--cache-dir",
			cacheDir,
			"--no-build",
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
