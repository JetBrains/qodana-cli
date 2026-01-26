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

package tooling

import (
	"os/exec"
	"strings"
	"testing"
)

func TestGetQodanaJBRPath(t *testing.T) {
	// Get the JBR path
	javaPath := GetQodanaJBRPath()

	if javaPath == "" {
		t.Fatal("Java path is empty")
	}

	t.Logf("Java executable path: %s", javaPath)

	// Execute java -version
	cmd := exec.Command(javaPath, "-version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to execute java -version: %v\nOutput: %s", err, string(output))
	}

	t.Logf("Java version output:\n%s", string(output))

	// Verify output contains expected content
	outputStr := string(output)

	if !strings.Contains(outputStr, "JDK Runtime Environment") {
		t.Error(`expected output to contain "JDK Runtime Environment"`)
	}

	if len(outputStr) == 0 {
		t.Error("java -version produced no output")
	}
}
