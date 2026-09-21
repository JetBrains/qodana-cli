// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package edict

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/edict/managed"
	"gopkg.in/yaml.v3"
)

func TestInstallManagedSkillsAlongsideLegacy(t *testing.T) {
	destination := t.TempDir()
	legacyNames, err := InstallSkills(destination)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(legacyNames, "managed") || slices.Contains(legacyNames, "edict_manager") {
		t.Fatalf("managed bundle exposed as legacy skill: %v", legacyNames)
	}
	if _, err := os.Stat(filepath.Join(destination, "managed")); !os.IsNotExist(err) {
		t.Fatalf("legacy installation copied managed bundle: %v", err)
	}
	legacyPath := filepath.Join(destination, "edict-next-run", "SKILL.md")
	legacy, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	names, err := InstallManagedSkills(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(names, "edict_manager") || !slices.Contains(names, "managed-edict-next-run") {
		t.Fatalf("missing discoverable manager or managed worker: %v", names)
	}
	after, err := os.ReadFile(legacyPath)
	if err != nil || string(after) != string(legacy) {
		t.Fatalf("managed installation changed legacy skill: %v", err)
	}
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(destination, name, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.SplitN(string(content), "---\n", 3)
		if len(parts) != 3 {
			t.Fatalf("%s has no frontmatter", name)
		}
		var metadata struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(parts[1]), &metadata); err != nil {
			t.Fatalf("%s metadata: %v", name, err)
		}
		if metadata.Name != name || metadata.Description == "" {
			t.Fatalf("%s discovery metadata mismatch: %+v", name, metadata)
		}
		policyBytes, err := os.ReadFile(filepath.Join(destination, name, "agents", "openai.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		var config struct {
			Policy struct {
				AllowImplicitInvocation *bool `yaml:"allow_implicit_invocation"`
			} `yaml:"policy"`
		}
		if err := yaml.Unmarshal(policyBytes, &config); err != nil {
			t.Fatal(err)
		}
		if config.Policy.AllowImplicitInvocation == nil || *config.Policy.AllowImplicitInvocation != (name == "edict_manager") {
			t.Fatalf("%s has incorrect entry-gate invocation policy", name)
		}
	}
	if _, err := os.ReadFile(filepath.Join(destination, "edict_manager", "references", "protocol.md")); err != nil {
		t.Fatalf("shared managed protocol was not installed: %v", err)
	}
}

func TestManagedSkillsMatchRegisteredPolicies(t *testing.T) {
	names, err := ManagedSkillNames()
	if err != nil {
		t.Fatal(err)
	}
	policies := managed.DefaultRegistry()
	if len(names) != len(policies) {
		t.Fatalf("%d managed skills but %d registered policies", len(names), len(policies))
	}
	for _, policy := range policies {
		name := policy.Name
		if name != "edict_manager" {
			name = "managed-" + name
		}
		if !slices.Contains(names, name) {
			t.Errorf("registered skill %s has no bundled managed skill", policy.Name)
		}
	}
}

func TestInstallManagedSkillRejectsUnregisteredNames(t *testing.T) {
	destination := t.TempDir()
	for _, name := range []string{"missing", "edict-next-run", "../edict_manager", "managed"} {
		if err := InstallManagedSkill(destination, name); err == nil {
			t.Errorf("installed invalid managed name %q", name)
		}
	}
	if err := InstallManagedSkill(destination, "edict_manager"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "edict_manager", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := InstallSkill(destination, "managed"); err == nil {
		t.Fatal("legacy single installer accepted managed bundle as a skill")
	}
}
