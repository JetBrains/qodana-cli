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
	"github.com/spf13/pflag"
	"os"
	"path/filepath"
	"testing"
)

func TestMergeSarifReports(t *testing.T) {
	t.Skip() // TODO: @dima fix this test
	if err := os.Setenv("QODANA_AUTOMATION_GUID", "00000000-0000-1000-8000-000000000000"); err != nil {
		t.Fail()
	}
	if err := os.Setenv("QODANA_REPORT_ID", "43210"); err != nil {
		t.Fail()
	}
	if err := os.Setenv("QODANA_JOB_URL", "joburl"); err != nil {
		t.Fail()
	}
	toolCode := "QDCL"
	toolDesc := "Qodana for C/C++ (CMake)"
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	testdataPath := filepath.Join(workingDir, "..", "testdata")
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

	err = CopyDir(filepath.Join(testdataPath, "merge"), filepath.Join(dir, "tmp"))
	if err != nil {
		t.Fatal(err)
	}

	opts := &QodanaOptions{
		ResultsDir: dir,
		ProjectDir: dir,
		LinterSpecific: &SarifTestOptions{
			linterInfo: &LinterInfo{
				ProductCode:   toolCode,
				LinterName:    toolDesc,
				LinterVersion: "",
			},
		},
	}
	_, err = MergeSarifReports(opts, "01234")
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
	if err := os.Unsetenv("QODANA_AUTOMATION_GUID"); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("QODANA_REPORT_ID"); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("QODANA_JOB_URL"); err != nil {
		t.Fatal(err)
	}
	// do comparison
	if string(expected) != string(actual) {
		t.Fatal("Files are not equal")
	}
}

type SarifTestOptions struct {
	linterInfo *LinterInfo
}

func (s SarifTestOptions) AddFlags(flags *pflag.FlagSet) {
}

func (s SarifTestOptions) GetMountInfo() *MountInfo {
	return nil
}

func (s SarifTestOptions) MountTools(tempPath string, mountPath string, o *QodanaOptions) (map[string]string, error) {
	return make(map[string]string), nil
}

func (s SarifTestOptions) GetInfo(o *QodanaOptions) *LinterInfo {
	return s.linterInfo
}

func (s SarifTestOptions) Setup(o *QodanaOptions) error {
	return nil
}

func (s SarifTestOptions) RunAnalysis(o *QodanaOptions) error {
	return nil
}
