package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/internal/testutil/needs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupProjectDir creates a nested directory hierarchy tmpDir/g/p/project and
// optionally places a clang-tidy config file at a given ancestor level.
// ancestorLevel: -1 = don't create, 0 = in projectDir, 1 = parent, 2 = grandparent, etc.
// configName defaults to ".clang-tidy" if empty.
//
// Caps the findClangTidyConfig walk at tmpDir via QODANA_CLANG_TIDY_SEARCH_ROOT
// so stray config files in the filesystem ancestry cannot influence the test.
func setupProjectDir(t *testing.T, ancestorLevel int, configName string) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv(clangTidySearchRootEnv, tmpDir)
	projectDir := filepath.Join(tmpDir, "g", "p", "project")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	if ancestorLevel >= 0 {
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
	checks, _ := runProcessConfigFull(t, projectDir, version, includes, excludes)
	return checks
}

func runProcessConfigFull(
	t *testing.T,
	projectDir string,
	version string,
	includes []qdyaml.Clude,
	excludes []qdyaml.Clude,
) (checks, configFile string) {
	t.Helper()
	ctx := thirdpartyscan.ContextBuilder{
		ProjectDir: projectDir,
		QodanaYamlConfig: thirdpartyscan.QodanaYamlConfig{
			Version:  version,
			Includes: includes,
			Excludes: excludes,
		},
	}.Build()
	checks, configFile, err := processConfig(ctx)
	require.NoError(t, err)
	return checks, configFile
}

// TestFindClangTidyConfig =====

func TestFindClangTidyConfig(t *testing.T) {
	t.Run(".clang-tidy in start directory", func(t *testing.T) {
		startDir := t.TempDir()
		t.Setenv(clangTidySearchRootEnv, startDir)
		require.NoError(t, os.WriteFile(filepath.Join(startDir, ".clang-tidy"), []byte("Checks: '*'\n"), 0o644))

		path, err := findClangTidyConfig(startDir)
		assert.NoError(t, err)
		assert.Equal(t, ".clang-tidy", filepath.Base(path))
	})

	t.Run("_clang-tidy in start directory", func(t *testing.T) {
		startDir := t.TempDir()
		t.Setenv(clangTidySearchRootEnv, startDir)
		require.NoError(t, os.WriteFile(filepath.Join(startDir, "_clang-tidy"), []byte("Checks: '*'\n"), 0o644))

		path, err := findClangTidyConfig(startDir)
		assert.NoError(t, err)
		assert.Equal(t, "_clang-tidy", filepath.Base(path))
	})

	t.Run(".clang-tidy in parent directory", func(t *testing.T) {
		parent := t.TempDir()
		t.Setenv(clangTidySearchRootEnv, parent)
		startDir := filepath.Join(parent, "project")
		require.NoError(t, os.MkdirAll(startDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(parent, ".clang-tidy"), []byte("Checks: '*'\n"), 0o644))

		path, err := findClangTidyConfig(startDir)
		assert.NoError(t, err)
		assert.Equal(t, ".clang-tidy", filepath.Base(path))
	})

	t.Run(".clang-tidy in grandparent directory", func(t *testing.T) {
		grandparent := t.TempDir()
		t.Setenv(clangTidySearchRootEnv, grandparent)
		startDir := filepath.Join(grandparent, "parent", "project")
		require.NoError(t, os.MkdirAll(startDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(grandparent, ".clang-tidy"), []byte("Checks: '*'\n"), 0o644))

		path, err := findClangTidyConfig(startDir)
		assert.NoError(t, err)
		assert.Equal(t, ".clang-tidy", filepath.Base(path))
	})

	t.Run("no config anywhere", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(clangTidySearchRootEnv, tmpDir)
		startDir := filepath.Join(tmpDir, "project")
		require.NoError(t, os.MkdirAll(startDir, 0o755))

		path, err := findClangTidyConfig(startDir)
		assert.NoError(t, err)
		assert.Empty(t, path)
	})

	t.Run("finds config AT the searchRoot directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		root := filepath.Join(tmpDir, "root")
		startDir := filepath.Join(root, "inside", "deeper")
		require.NoError(t, os.MkdirAll(startDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, ".clang-tidy"), []byte("Checks: '*'\n"), 0o644))
		t.Setenv(clangTidySearchRootEnv, root)

		path, err := findClangTidyConfig(startDir)
		assert.NoError(t, err)
		assert.Equal(t, ".clang-tidy", filepath.Base(path))
		resolvedRoot, err := resolvePath(root)
		require.NoError(t, err)
		assert.Equal(t, resolvedRoot, filepath.Dir(path))
	})

	t.Run("does not find config ABOVE the searchRoot directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		above := filepath.Join(tmpDir, "above")
		root := filepath.Join(above, "root")
		startDir := filepath.Join(root, "inside", "deeper")
		require.NoError(t, os.MkdirAll(startDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(above, ".clang-tidy"), []byte("Checks: '*'\n"), 0o644))
		t.Setenv(clangTidySearchRootEnv, root)

		path, err := findClangTidyConfig(startDir)
		assert.NoError(t, err)
		assert.Empty(t, path)
	})

	t.Run("prefers .clang-tidy over _clang-tidy in same dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(clangTidySearchRootEnv, tmpDir)
		dual := filepath.Join(tmpDir, "dual")
		require.NoError(t, os.MkdirAll(dual, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dual, ".clang-tidy"), []byte("Checks: '*'\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dual, "_clang-tidy"), []byte("Checks: '*'\n"), 0o644))

		path, err := findClangTidyConfig(dual)
		assert.NoError(t, err)
		assert.Equal(t, ".clang-tidy", filepath.Base(path))
	})

	t.Run("errors when searchRoot is not an ancestor of startDir", func(t *testing.T) {
		tmpDir := t.TempDir()
		a := filepath.Join(tmpDir, "a")
		b := filepath.Join(tmpDir, "b")
		require.NoError(t, os.MkdirAll(a, 0o755))
		require.NoError(t, os.MkdirAll(b, 0o755))
		t.Setenv(clangTidySearchRootEnv, a)

		_, err := findClangTidyConfig(b)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not an ancestor")
	})

	t.Run("errors when searchRoot is a descendant of startDir", func(t *testing.T) {
		tmpDir := t.TempDir()
		parent := filepath.Join(tmpDir, "parent")
		child := filepath.Join(parent, "child")
		require.NoError(t, os.MkdirAll(child, 0o755))
		t.Setenv(clangTidySearchRootEnv, child)

		_, err := findClangTidyConfig(parent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not an ancestor")
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

// TestProcessConfig — configFile return value =====

func TestProcessConfig_ConfigFile_DotClangTidyAtProjectRoot(t *testing.T) {
	dir := setupProjectDir(t, 0, ".clang-tidy")
	_, configFile := runProcessConfigFull(t, dir, "", nil, nil)
	assert.Empty(t, configFile, ".clang-tidy relies on clang-tidy's native walk")
}

func TestProcessConfig_ConfigFile_DotClangTidyInParent(t *testing.T) {
	dir := setupProjectDir(t, 1, ".clang-tidy")
	_, configFile := runProcessConfigFull(t, dir, "", nil, nil)
	assert.Empty(t, configFile, ".clang-tidy in parent still relies on clang-tidy's native walk")
}

func TestProcessConfig_ConfigFile_UnderscoreClangTidyAtProjectRoot(t *testing.T) {
	dir := setupProjectDir(t, 0, "_clang-tidy")
	_, configFile := runProcessConfigFull(t, dir, "", nil, nil)
	require.NotEmpty(t, configFile)
	assert.Equal(t, "_clang-tidy", filepath.Base(configFile))
	resolvedDir, err := resolvePath(dir)
	require.NoError(t, err)
	assert.Equal(t, resolvedDir, filepath.Dir(configFile))
}

func TestProcessConfig_ConfigFile_UnderscoreClangTidyInParent(t *testing.T) {
	dir := setupProjectDir(t, 1, "_clang-tidy")
	_, configFile := runProcessConfigFull(t, dir, "", nil, nil)
	require.NotEmpty(t, configFile)
	assert.Equal(t, "_clang-tidy", filepath.Base(configFile))
}

func TestProcessConfig_ConfigFile_NoConfig(t *testing.T) {
	dir := setupProjectDir(t, -1, "")
	_, configFile := runProcessConfigFull(t, dir, "", nil, nil)
	assert.Empty(t, configFile)
}

func TestProcessConfig_ConfigFile_UnderscoreClangTidyWithIncludes(t *testing.T) {
	dir := setupProjectDir(t, 0, "_clang-tidy")
	checks, configFile := runProcessConfigFull(t, dir, "1.0", []qdyaml.Clude{{Name: "bugprone-*"}}, nil)
	assert.Equal(t, "--checks=bugprone-*", checks, "overrides layer on top of config")
	require.NotEmpty(t, configFile)
	assert.Equal(t, "_clang-tidy", filepath.Base(configFile))
}

func TestProcessConfig_ConfigFile_DotClangTidyWithIncludes(t *testing.T) {
	dir := setupProjectDir(t, 0, ".clang-tidy")
	checks, configFile := runProcessConfigFull(t, dir, "1.0", []qdyaml.Clude{{Name: "bugprone-*"}}, nil)
	assert.Equal(t, "--checks=bugprone-*", checks)
	assert.Empty(t, configFile, ".clang-tidy must not set configFile even with overrides")
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

// TestDefaultChecks_AcceptedByClangTidy is a smoke test that the curated
// defaultChecks string is accepted by the real (embedded) clang-tidy. Runs
// clang-tidy --list-checks --checks=<defaultChecks> and confirms:
//   - a few expected category names appear in stdout,
//   - stderr carries no "unknown check" warnings (typo guard).
func TestDefaultChecks_AcceptedByClangTidy(t *testing.T) {
	needs.Need(t, needs.ClangDeps)

	tmpDir := t.TempDir()
	mountInfo, err := ClangLinter{}.MountTools(tmpDir)
	require.NoError(t, err, "mounting the embedded clang-tidy must succeed when ClangDeps is on")
	bin := mountInfo[thirdpartyscan.Clang]

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, "--list-checks", "--checks="+defaultChecks)
	// Force English output so the typo-guard substring match below is locale-stable.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("clang-tidy rejected defaultChecks: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{"bugprone-", "performance-", "readability-"} {
		assert.Contains(t, out, want, "enabled check list should contain %q", want)
	}
	errOut := stderr.String()
	assert.NotContains(t, errOut, "unknown check",
		"defaultChecks must not reference unknown checks; stderr: %s", errOut)
	assert.NotContains(t, errOut, "does not exist",
		"defaultChecks must not reference non-existent checks; stderr: %s", errOut)
}
