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
	"strings"
)

//go:embed all:skills
var skillsFS embed.FS

const skillsRoot = "skills"
const managedSkillsRoot = skillsRoot + "/managed"

// SkillNames returns the bundled legacy skill names, excluding the managed bundle.
func SkillNames() ([]string, error) {
	entries, err := skillsFS.ReadDir(skillsRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read bundled skills: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "managed" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// InstallSkills copies the bundled legacy skills into destDir (one subdirectory per skill),
// overwriting existing files. Returns the names of the installed skills.
func InstallSkills(destDir string) ([]string, error) {
	names, err := SkillNames()
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if err := installBundledSkill(destDir, skillsRoot, name, name); err != nil {
			return nil, err
		}
	}
	return names, nil
}

// InstallSkill copies one bundled skill into destDir/<name>, overwriting existing files.
func InstallSkill(destDir string, name string) error {
	names, err := SkillNames()
	if err != nil {
		return err
	}
	found := false
	for _, available := range names {
		if available == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown bundled skill %q", name)
	}
	return installBundledSkill(destDir, skillsRoot, name, name)
}

// ManagedSkillNames returns the discoverable names of the managed skills. Their
// installed names are distinct from the legacy skills; the MCP registry continues
// to use the original edict-next-* IDs for managed children.
func ManagedSkillNames() ([]string, error) {
	entries, err := skillsFS.ReadDir(managedSkillsRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read bundled managed skills: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name != "edict_manager" {
			name = "managed-" + name
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// InstallManagedSkills installs the manager and all managed children together.
// InstallSkills intentionally installs only the legacy, unmanaged skill set.
func InstallManagedSkills(destDir string) ([]string, error) {
	names, err := ManagedSkillNames()
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if err := installBundledSkill(destDir, managedSkillsRoot, strings.TrimPrefix(name, "managed-"), name); err != nil {
			return nil, err
		}
	}
	return names, nil
}

// InstallManagedSkill installs one managed skill by its discoverable name.
// Prefer InstallManagedSkills when the manager needs its complete call graph.
func InstallManagedSkill(destDir, name string) error {
	names, err := ManagedSkillNames()
	if err != nil {
		return err
	}
	for _, available := range names {
		if available == name {
			return installBundledSkill(destDir, managedSkillsRoot, strings.TrimPrefix(name, "managed-"), name)
		}
	}
	return fmt.Errorf("unknown bundled managed skill %q", name)
}

func installBundledSkill(destDir, root, sourceName, name string) error {
	sourceRoot := filepath.ToSlash(filepath.Join(root, sourceName))
	err := fs.WalkDir(skillsFS, sourceRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(filepath.FromSlash(sourceRoot), filepath.FromSlash(path))
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, name, relative)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		content, err := skillsFS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, 0o644)
	})
	if err != nil {
		return fmt.Errorf("failed to install skill %s to %s: %w", name, destDir, err)
	}
	return nil
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
