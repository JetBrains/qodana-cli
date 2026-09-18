/*
 * Copyright 2021-2026 JetBrains s.r.o.
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

package edict

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const localBazelQodanaRunnerSkillName = "qodana-local-bazel-runner"

const localBazelQodanaRunnerSkill = `---
name: qodana-local-bazel-runner
description: Run a JVM Qodana analysis from a local Ultimate checkout configured by ULTIMATE_EDICT_REPO and return its SARIF report. Use for local integration tests that supply a project and result directory, optionally with a baseline and absent-result collection.
---

# Local Bazel Qodana Runner

Accept a project directory and an empty result directory. Optionally accept a baseline SARIF path
and an include-absent flag. Require ULTIMATE_EDICT_REPO to name an existing Ultimate checkout
containing bazel.cmd. Do not discover or accept another checkout path.

Run from that checkout. For a normal analysis execute:

    /bin/sh ./bazel.cmd run //build:qodana_for_jvm -- qodana <project> <result-directory>

For a baseline analysis with include-absent enabled execute:

    /bin/sh ./bazel.cmd run //build:qodana_for_jvm -- qodana --baseline <baseline-sarif> --baseline-include-absent <project> <result-directory>

Do not change the project, inspection setup, baseline, or requested result directory. Require a zero
exit status, then parse <result-directory>/qodana.sarif.json as JSON and return its absolute path to
the calling skill. A command failure or missing/invalid report is a failed run.
`

func requireUltimateEdictRepository(t *testing.T) string {
	t.Helper()
	configured := strings.TrimSpace(os.Getenv("ULTIMATE_EDICT_REPO"))
	if configured == "" {
		t.Skip("ULTIMATE_EDICT_REPO is required for the local Bazel Qodana integration test")
	}
	repository, err := filepath.Abs(configured)
	if err != nil {
		t.Fatalf("resolve ULTIMATE_EDICT_REPO: %v", err)
	}
	requireDirectory(t, repository)
	requireFile(t, filepath.Join(repository, "bazel.cmd"))
	return repository
}

func installLocalBazelQodanaRunnerSkill(t *testing.T, skillsDirectory string) {
	t.Helper()
	directory := filepath.Join(skillsDirectory, localBazelQodanaRunnerSkillName)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(localBazelQodanaRunnerSkill), 0o644); err != nil {
		t.Fatal(err)
	}
}
