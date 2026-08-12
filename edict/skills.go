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

// Package edict bundles the edict agent skills shipped with the Qodana CLI.
package edict

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

//go:embed all:skills
var skillsFS embed.FS

const skillsRoot = "skills"

// SkillNames returns the names of the bundled skills (top-level directories under skills/).
func SkillNames() ([]string, error) {
	entries, err := skillsFS.ReadDir(skillsRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read bundled skills: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// InstallSkills copies all bundled skills into destDir (one subdirectory per skill),
// overwriting existing files. Returns the names of the installed skills.
func InstallSkills(destDir string) ([]string, error) {
	names, err := SkillNames()
	if err != nil {
		return nil, err
	}
	err = fs.WalkDir(
		skillsFS, skillsRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(skillsRoot, filepath.FromSlash(path))
			if err != nil {
				return err
			}
			target := filepath.Join(destDir, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			content, err := skillsFS.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, content, 0o644)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to install skills to %s: %w", destDir, err)
	}
	return names, nil
}

// CodexSkillsDir resolves the Codex CLI skills directory.
// Project-level: <projectDir>/.codex/skills. User-level: $CODEX_HOME/skills,
// falling back to ~/.codex/skills when CODEX_HOME is not set.
func CodexSkillsDir(projectLevel bool, projectDir string) (string, error) {
	if projectLevel {
		abs, err := filepath.Abs(projectDir)
		if err != nil {
			return "", err
		}
		return filepath.Join(abs, ".codex", "skills"), nil
	}
	if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" {
		return filepath.Join(codexHome, "skills"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home directory: %w", err)
	}
	return filepath.Join(home, ".codex", "skills"), nil
}
