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
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFetchPublisher(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(tempDir) // clean up
	path := filepath.Join(tempDir, "publisher.jar")
	fetchPublisher(path)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("fetchPublisher() failed, expected %v to exists, got error: %v", path, err)
	}
}

func TestGetPublisherArgs(t *testing.T) {
	// Set up test options
	publisher := Publisher{
		ResultsDir: "/path/to/results",
		ProjectDir: "/path/to/project",
		LogDir:     "",
		AnalysisId: "test-analysis-id",
	}

	// Set up test environment variables
	err := os.Setenv(qdenv.QodanaToolEnv, "test-tool")
	if err != nil {
		t.Fatal(err)
	}

	java, _ := utils.GetJavaExecutablePath()
	// Call the function being tested
	publisherArgs := getPublisherArgs(java, "test-publisher.jar", publisher, "test-token", "test-endpoint")

	// Assert that the expected arguments are present
	expectedArgs := []string{
		java,
		"-jar",
		"test-publisher.jar",
		"--analysis-id", "test-analysis-id",
		"--sources-path", "/path/to/project",
		"--report-path", filepath.FromSlash("/path/to/results/results"),
		"--token", "test-token",
		"--tool", "test-tool",
		"--endpoint", "test-endpoint",
	}
	if !reflect.DeepEqual(publisherArgs, expectedArgs) {
		t.Errorf("getPublisherArgs returned incorrect arguments: got %v, expected %v", publisherArgs, expectedArgs)
	}
}
