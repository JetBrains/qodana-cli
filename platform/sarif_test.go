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

package platform

import (
	"github.com/JetBrains/qodana-cli/v2024/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeSarifReports(t *testing.T) {
	if err := os.Setenv("QODANA_AUTOMATION_GUID", "00000000-0000-1000-8000-000000000000"); err != nil {
		t.Fail()
	}
	if err := os.Setenv("QODANA_REPORT_ID", "43210"); err != nil {
		t.Fail()
	}
	if err := os.Setenv("QODANA_JOB_URL", "joburl"); err != nil {
		t.Fail()
	}
	if err := os.Setenv("QODANA_REMOTE_URL", "repourl"); err != nil {
		t.Fail()
	}
	if err := os.Setenv("QODANA_BRANCH", "foo"); err != nil {
		t.Fail()
	}
	if err := os.Setenv("QODANA_REVISION", "bar"); err != nil {
		t.Fail()
	}
	toolCode := "QDCL"
	toolDesc := "Qodana for C/C++ (CMake)"
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	testdataPath := filepath.Join(workingDir, "testdata")
	// create temp directory
	dir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(dir)
	err = os.Mkdir(filepath.Join(dir, "tmp"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = utils.CopyDir(filepath.Join(testdataPath, "merge"), filepath.Join(dir, "tmp"))
	if err != nil {
		t.Fatal(err)
	}

	context := thirdpartyscan.ContextBuilder{
		ProjectDir: dir,
		ResultsDir: dir,
		LinterInfo: thirdpartyscan.LinterInfo{
			ProductCode:   toolCode,
			LinterName:    toolDesc,
			LinterVersion: "",
		},
	}.Build()

	_, err = MergeSarifReports(context, "01234")
	if err != nil {
		t.Fatal(err)
	}
	// check if file exists
	_, err = os.Stat(filepath.Join(dir, "qodana.sarif.json"))
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatal("Resulting SARIF file not found")
		}
	}
	actual, err := os.ReadFile(filepath.Join(dir, "qodana.sarif.json"))
	if err != nil {
		t.Fatal(err)
	}
	// compare with expected file
	expected, err := os.ReadFile(filepath.Join(testdataPath, "merged.qodana.sarif.json"))
	if err != nil {
		t.Fatal(err)
	}
	envs := []string{
		"QODANA_AUTOMATION_GUID",
		"QODANA_REPORT_ID",
		"QODANA_JOB_URL",
		"QODANA_REMOTE_URL",
		"QODANA_BRANCH",
		"QODANA_REVISION",
	}

	for _, env := range envs {
		if err := os.Unsetenv(env); err != nil {
			t.Fatalf("Failed to unset environment variable %s: %v", env, err)
		}
	}
	// do comparison
	expString := normalize(string(expected))
	actString := normalize(string(actual))

	if expString != actString {
		t.Fatalf("Files are not of equal. Length: expected %d vs actual %d", len(expString), len(actString))
	}
}

func normalize(s string) string {
	return strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(s)
}
