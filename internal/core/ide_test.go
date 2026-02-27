package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/core/corescan"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/internal/testutil/mockexe"
	"github.com/JetBrains/qodana-cli/internal/testutil/mocklinter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockIdeHandler is a handler that behaves like a real IDE: it writes
// qodana-short.sarif.json to the results directory (the last argv element).
func mockIdeHandler(ctx *mockexe.CallContext) int {
	resultsDir := ctx.Argv[len(ctx.Argv)-1]
	sarifContent := `{"runs": [{"invocations": [{"exitCode": 0}]}]}`
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		return 1
	}
	if err := os.WriteFile(
		filepath.Join(resultsDir, "qodana-short.sarif.json"),
		[]byte(sarifContent), 0o644,
	); err != nil {
		return 1
	}
	return 0
}

func TestInstallPlugins(t *testing.T) {
	exePath := mocklinter.Native(t, func(ctx *mockexe.CallContext) int {
		require.Contains(ctx.T, ctx.Argv, "installPlugins")
		require.Contains(ctx.T, ctx.Argv, "test-plugin")
		return 0
	})

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	assert.NoError(t, os.MkdirAll(configDir, 0o755))

	ctx := corescan.ContextBuilder{
		Analyser: mocklinter.Linter.NativeAnalyzer(),
		Prod: product.Product{
			IdeScript:      exePath,
			BaseScriptName: product.Idea,
			Code:           mocklinter.Linter.ProductCode,
			Build:          mocklinter.MockBuild,
			Version:        mocklinter.MockVersion,
		},
		ConfigDir: configDir,
		LogDir:    filepath.Join(tmpDir, "log"),
		CacheDir:  filepath.Join(tmpDir, "cache"),
		QodanaYamlConfig: corescan.QodanaYamlConfig{
			Plugins: []qdyaml.Plugin{{Id: "test-plugin"}},
		},
	}.Build()

	assert.NoError(t, installPlugins(ctx))
}

func TestInstallPlugins_PathWithSpaces(t *testing.T) {
	tmpDir := t.TempDir()

	// Place mock IDE in a directory with spaces.
	dirWithSpaces := filepath.Join(tmpDir, "My IDE")
	handler := func(ctx *mockexe.CallContext) int {
		require.Contains(ctx.T, ctx.Argv, "installPlugins")
		require.Contains(ctx.T, ctx.Argv, "test-plugin")
		return 0
	}
	ideScript := mockexe.CreateMockExe(t, filepath.Join(dirWithSpaces, "idea"), handler)

	configDir := filepath.Join(tmpDir, "my config")
	assert.NoError(t, os.MkdirAll(configDir, 0o755))

	ctx := corescan.ContextBuilder{
		Analyser: mocklinter.Linter.NativeAnalyzer(),
		Prod: product.Product{
			IdeScript:      ideScript,
			BaseScriptName: product.Idea,
			Code:           mocklinter.Linter.ProductCode,
			Build:          mocklinter.MockBuild,
		},
		ConfigDir: configDir,
		LogDir:    filepath.Join(tmpDir, "my log"),
		CacheDir:  filepath.Join(tmpDir, "my cache"),
		QodanaYamlConfig: corescan.QodanaYamlConfig{
			Plugins: []qdyaml.Plugin{{Id: "test-plugin"}},
		},
	}.Build()

	assert.NoError(t, installPlugins(ctx))
}

func TestRunQodanaLocal(t *testing.T) {
	exePath := mocklinter.Native(t, mockIdeHandler)

	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	resultsDir := filepath.Join(tmpDir, "results")
	configDir := filepath.Join(tmpDir, "config")
	cacheDir := filepath.Join(tmpDir, "cache")

	for _, dir := range []string{projectDir, resultsDir, configDir, cacheDir} {
		assert.NoError(t, os.MkdirAll(dir, 0o755))
	}

	ctx := corescan.ContextBuilder{
		Analyser: mocklinter.Linter.NativeAnalyzer(),
		Prod: product.Product{
			IdeScript:      exePath,
			BaseScriptName: product.Idea,
			Code:           mocklinter.Linter.ProductCode,
			Build:          mocklinter.MockBuild,
			Version:        mocklinter.MockVersion,
		},
		ProjectDir: projectDir,
		ResultsDir: resultsDir,
		ConfigDir:  configDir,
		CacheDir:   cacheDir,
		LogDir:     filepath.Join(tmpDir, "log"),
	}.Build()

	exitCode, err := runQodanaLocal(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCode)
}

func TestRunQodanaLocal_PathWithSpaces(t *testing.T) {
	tmpDir := t.TempDir()

	// Build mock IDE in a directory with spaces — requires manual Product
	// construction because mocklinter.Native() picks its own temp path.
	dirWithSpaces := filepath.Join(tmpDir, "My IDE")
	ideScript := mockexe.CreateMockExe(t, filepath.Join(dirWithSpaces, "idea"), mockIdeHandler)

	projectDir := filepath.Join(tmpDir, "my project")
	resultsDir := filepath.Join(tmpDir, "my results")
	configDir := filepath.Join(tmpDir, "my config")
	cacheDir := filepath.Join(tmpDir, "my cache")

	for _, dir := range []string{projectDir, resultsDir, configDir, cacheDir} {
		assert.NoError(t, os.MkdirAll(dir, 0o755))
	}

	ctx := corescan.ContextBuilder{
		Analyser: mocklinter.Linter.NativeAnalyzer(),
		Prod: product.Product{
			IdeScript:      ideScript,
			BaseScriptName: product.Idea,
			Code:           mocklinter.Linter.ProductCode,
			Build:          mocklinter.MockBuild,
			Version:        mocklinter.MockVersion,
		},
		ProjectDir: projectDir,
		ResultsDir: resultsDir,
		ConfigDir:  configDir,
		CacheDir:   cacheDir,
		LogDir:     filepath.Join(tmpDir, "my log"),
	}.Build()

	exitCode, err := runQodanaLocal(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCode)
}
