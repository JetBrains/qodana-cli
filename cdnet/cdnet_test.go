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
	"github.com/JetBrains/qodana-cli/v2024/platform"
	platformcmd "github.com/JetBrains/qodana-cli/v2024/platform/cmd"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinterRun(t *testing.T) {
	// skip test on GH, since required jars are not there
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip()
	}
	projectPath := createNativeProject(t, "badrules")
	defer deferredCleanup(projectPath)
	outputDir := filepath.Join(os.TempDir(), "cdnet-output")
	defer deferredCleanup(outputDir)

	options := platform.DefineOptions(
		func() platform.ThirdPartyLinter {
			return &CdnetLinter{
				LinterInfo: &platform.LinterInfo{
					ProductCode:   productCode,
					LinterName:    linterName,
					LinterVersion: "2023.3",
					IsEap:         true,
				},
			}
		},
	)

	command := platformcmd.NewScanCommand(options)
	command.SetArgs([]string{"-i", projectPath, "-o", outputDir, "--no-build"})
	err := command.Execute()
	defer deferredCleanup(options.CacheDir)
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
	err = gitClone("https://github.com/hybloid/BadRulesProject", location)
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
