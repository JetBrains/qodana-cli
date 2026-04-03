package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireNoClangTidyInAncestors verifies the test environment doesn't have
// a stray .clang-tidy or _clang-tidy in the filesystem ancestry of dir.
func requireNoClangTidyInAncestors(t *testing.T, dir string) {
	t.Helper()
	found, err := findClangTidyConfig(dir)
	require.NoError(t, err)
	if found {
		t.Skipf("Skipping: found .clang-tidy or _clang-tidy in ancestry of %s (environment issue)", dir)
	}
}

// setupProjectDir creates a nested directory hierarchy tmpDir/g/p/project and
// optionally places a clang-tidy config file at a given ancestor level.
// ancestorLevel: -1 = don't create, 0 = in projectDir, 1 = parent, 2 = grandparent, etc.
// configName defaults to ".clang-tidy" if empty.
func setupProjectDir(t *testing.T, ancestorLevel int, configName string) string {
	t.Helper()
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "g", "p", "project")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	if ancestorLevel < 0 {
		requireNoClangTidyInAncestors(t, tmpDir)
	} else {
		target := projectDir
		for range ancestorLevel {
			target = filepath.Dir(target)
		}
		if configName == "" {
			configName = ".clang-tidy"
		}
		require.NoError(t, os.WriteFile(
			filepath.Join(target, configName),
			[]byte("Checks: '-*,bugprone-*'\n"), 0o644,
		))
	}
	return projectDir
}

func runProcessConfig(
	t *testing.T,
	projectDir string,
	version string,
	includes []qdyaml.Clude,
	excludes []qdyaml.Clude,
) string {
	t.Helper()
	ctx := thirdpartyscan.ContextBuilder{
		ProjectDir: projectDir,
		QodanaYamlConfig: thirdpartyscan.QodanaYamlConfig{
			Version:  version,
			Includes: includes,
			Excludes: excludes,
		},
	}.Build()
	result, err := processConfig(ctx)
	require.NoError(t, err)
	return result
}

// TestFindClangTidyConfig =====

func TestFindClangTidyConfig(t *testing.T) {
	t.Run(".clang-tidy in start directory", func(t *testing.T) {
		startDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(startDir, ".clang-tidy"), []byte("Checks: '*'\n"), 0o644))

		found, err := findClangTidyConfig(startDir)
		assert.NoError(t, err)
		assert.True(t, found)
	})

	t.Run("_clang-tidy in start directory", func(t *testing.T) {
		startDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(startDir, "_clang-tidy"), []byte("Checks: '*'\n"), 0o644))

		found, err := findClangTidyConfig(startDir)
		assert.NoError(t, err)
		assert.True(t, found)
	})

	t.Run(".clang-tidy in parent directory", func(t *testing.T) {
		parent := t.TempDir()
		startDir := filepath.Join(parent, "project")
		require.NoError(t, os.MkdirAll(startDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(parent, ".clang-tidy"), []byte("Checks: '*'\n"), 0o644))

		found, err := findClangTidyConfig(startDir)
		assert.NoError(t, err)
		assert.True(t, found)
	})

	t.Run(".clang-tidy in grandparent directory", func(t *testing.T) {
		grandparent := t.TempDir()
		startDir := filepath.Join(grandparent, "parent", "project")
		require.NoError(t, os.MkdirAll(startDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(grandparent, ".clang-tidy"), []byte("Checks: '*'\n"), 0o644))

		found, err := findClangTidyConfig(startDir)
		assert.NoError(t, err)
		assert.True(t, found)
	})

	t.Run("no config anywhere", func(t *testing.T) {
		tmpDir := t.TempDir()
		startDir := filepath.Join(tmpDir, "project")
		require.NoError(t, os.MkdirAll(startDir, 0o755))
		requireNoClangTidyInAncestors(t, tmpDir)

		found, err := findClangTidyConfig(startDir)
		assert.NoError(t, err)
		assert.False(t, found)
	})
}

// TestProcessConfig — no .clang-tidy (defaults are the base) =====

func TestProcessConfig_NoConfig_Defaults(t *testing.T) {
	dir := setupProjectDir(t, -1, "")
	assert.Equal(t, "--checks="+defaultChecks, runProcessConfig(t, dir, "", nil, nil))
}

func TestProcessConfig_NoConfig_OnlyIncludes(t *testing.T) {
	dir := setupProjectDir(t, -1, "")
	result := runProcessConfig(t, dir, "1.0", []qdyaml.Clude{{Name: "bugprone-*"}}, nil)
	assert.Equal(t, "--checks="+defaultChecks+",bugprone-*", result)
}

func TestProcessConfig_NoConfig_OnlyExcludes(t *testing.T) {
	dir := setupProjectDir(t, -1, "")
	result := runProcessConfig(t, dir, "1.0", nil, []qdyaml.Clude{{Name: "clang-analyzer-cplusplus.NewDeleteLeaks"}})
	assert.Equal(t, "--checks="+defaultChecks+",-clang-analyzer-cplusplus.NewDeleteLeaks", result)
}

func TestProcessConfig_NoConfig_BothIncludesAndExcludes(t *testing.T) {
	dir := setupProjectDir(t, -1, "")
	result := runProcessConfig(t, dir, "1.0",
		[]qdyaml.Clude{{Name: "bugprone-*"}},
		[]qdyaml.Clude{{Name: "bugprone-argument-comment"}},
	)
	assert.Equal(t, "--checks="+defaultChecks+",bugprone-*,-bugprone-argument-comment", result)
}

func TestProcessConfig_NoConfig_MultipleExcludes(t *testing.T) {
	dir := setupProjectDir(t, -1, "")
	result := runProcessConfig(t, dir, "1.0", nil, []qdyaml.Clude{
		{Name: "clang-analyzer-cplusplus.NewDeleteLeaks"},
		{Name: "clang-analyzer-core.uninitialized.ArraySubscript"},
		{Name: "clang-analyzer-core.NullDereference"},
	})
	assert.Equal(t, "--checks="+defaultChecks+",-clang-analyzer-cplusplus.NewDeleteLeaks,-clang-analyzer-core.uninitialized.ArraySubscript,-clang-analyzer-core.NullDereference", result)
}

func TestProcessConfig_NoConfig_MultipleIncludes(t *testing.T) {
	dir := setupProjectDir(t, -1, "")
	result := runProcessConfig(t, dir, "1.0",
		[]qdyaml.Clude{{Name: "bugprone-*"}, {Name: "performance-*"}}, nil,
	)
	assert.Equal(t, "--checks="+defaultChecks+",bugprone-*,performance-*", result)
}

// TestProcessConfig — .clang-tidy in project dir (config is the base) =====

func TestProcessConfig_WithConfig_DefersToConfig(t *testing.T) {
	dir := setupProjectDir(t, 0, "")
	assert.Equal(t, "", runProcessConfig(t, dir, "", nil, nil))
}

func TestProcessConfig_WithConfig_OnlyIncludes(t *testing.T) {
	dir := setupProjectDir(t, 0, "")
	result := runProcessConfig(t, dir, "1.0", []qdyaml.Clude{{Name: "bugprone-*"}}, nil)
	assert.Equal(t, "--checks=bugprone-*", result)
}

func TestProcessConfig_WithConfig_OnlyExcludes(t *testing.T) {
	dir := setupProjectDir(t, 0, "")
	result := runProcessConfig(t, dir, "1.0", nil, []qdyaml.Clude{{Name: "clang-analyzer-cplusplus.NewDeleteLeaks"}})
	assert.Equal(t, "--checks=-clang-analyzer-cplusplus.NewDeleteLeaks", result)
}

func TestProcessConfig_WithConfig_BothIncludesAndExcludes(t *testing.T) {
	dir := setupProjectDir(t, 0, "")
	result := runProcessConfig(t, dir, "1.0",
		[]qdyaml.Clude{{Name: "bugprone-*"}},
		[]qdyaml.Clude{{Name: "bugprone-argument-comment"}},
	)
	assert.Equal(t, "--checks=bugprone-*,-bugprone-argument-comment", result)
}

// TestProcessConfig — .clang-tidy in parent/grandparent =====

func TestProcessConfig_ConfigInParent_DefersToConfig(t *testing.T) {
	dir := setupProjectDir(t, 1, "")
	assert.Equal(t, "", runProcessConfig(t, dir, "", nil, nil))
}

func TestProcessConfig_ConfigInGrandparent_DefersToConfig(t *testing.T) {
	dir := setupProjectDir(t, 2, "")
	assert.Equal(t, "", runProcessConfig(t, dir, "", nil, nil))
}

func TestProcessConfig_ConfigInParent_OnlyExcludes(t *testing.T) {
	dir := setupProjectDir(t, 1, "")
	result := runProcessConfig(t, dir, "1.0", nil, []qdyaml.Clude{{Name: "clang-analyzer-cplusplus.NewDeleteLeaks"}})
	assert.Equal(t, "--checks=-clang-analyzer-cplusplus.NewDeleteLeaks", result)
}

func TestProcessConfig_ConfigInParent_OnlyIncludes(t *testing.T) {
	dir := setupProjectDir(t, 1, "")
	result := runProcessConfig(t, dir, "1.0", []qdyaml.Clude{{Name: "bugprone-*"}}, nil)
	assert.Equal(t, "--checks=bugprone-*", result)
}

func TestProcessConfig_ConfigInParent_IncludesAndExcludes(t *testing.T) {
	dir := setupProjectDir(t, 1, "")
	result := runProcessConfig(t, dir, "1.0",
		[]qdyaml.Clude{{Name: "bugprone-*"}},
		[]qdyaml.Clude{{Name: "bugprone-argument-comment"}},
	)
	assert.Equal(t, "--checks=bugprone-*,-bugprone-argument-comment", result)
}

// TestProcessConfig — _clang-tidy variant =====

func TestProcessConfig_UnderscoreClangTidy_DefersToConfig(t *testing.T) {
	dir := setupProjectDir(t, 1, "_clang-tidy")
	assert.Equal(t, "", runProcessConfig(t, dir, "", nil, nil))
}

// TestProcessConfig — filtering edge cases without config =====

func TestProcessConfig_ClionFiltered_FallsBackToDefaults(t *testing.T) {
	dir := setupProjectDir(t, -1, "")
	result := runProcessConfig(t, dir, "1.0", []qdyaml.Clude{{Name: "clion-misra-cpp2008-0-1-1"}}, nil)
	assert.Equal(t, "--checks="+defaultChecks, result)
}

func TestProcessConfig_ClionFiltered_WithExcludes(t *testing.T) {
	dir := setupProjectDir(t, -1, "")
	result := runProcessConfig(t, dir, "1.0",
		[]qdyaml.Clude{{Name: "clion-misra-cpp2008-0-1-1"}},
		[]qdyaml.Clude{{Name: "clang-analyzer-cplusplus.NewDeleteLeaks"}},
	)
	assert.Equal(t, "--checks="+defaultChecks+",-clang-analyzer-cplusplus.NewDeleteLeaks", result)
}

func TestProcessConfig_MixedValidAndClionIncludes(t *testing.T) {
	dir := setupProjectDir(t, -1, "")
	result := runProcessConfig(t, dir, "1.0",
		[]qdyaml.Clude{{Name: "clion-misra-cpp2008-0-1-1"}, {Name: "bugprone-*"}}, nil,
	)
	assert.Equal(t, "--checks="+defaultChecks+",bugprone-*", result)
}

func TestProcessConfig_QuotedIncludesFiltered_FallsBackToDefaults(t *testing.T) {
	dir := setupProjectDir(t, -1, "")
	result := runProcessConfig(t, dir, "1.0", []qdyaml.Clude{{Name: `bugprone-"test"`}}, nil)
	assert.Equal(t, "--checks="+defaultChecks, result)
}

func TestProcessConfig_QuotedExcludesFiltered_FallsBackToDefaults(t *testing.T) {
	dir := setupProjectDir(t, -1, "")
	result := runProcessConfig(t, dir, "1.0", nil, []qdyaml.Clude{{Name: `clang-analyzer-"test"`}})
	assert.Equal(t, "--checks="+defaultChecks, result)
}

// TestProcessConfig — filtering edge cases with config =====

func TestProcessConfig_AllIncludesFiltered_WithConfig_DefersToConfig(t *testing.T) {
	dir := setupProjectDir(t, 0, "")
	result := runProcessConfig(t, dir, "1.0", []qdyaml.Clude{{Name: "clion-misra-cpp2008-0-1-1"}}, nil)
	assert.Equal(t, "", result)
}
