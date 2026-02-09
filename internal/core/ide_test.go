package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/core/corescan"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/stretchr/testify/assert"
)

func TestInstallPlugins(t *testing.T) {
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS == "windows" {
		t.Skip("test uses a shell script as a fake IDE")
	}

	tmpDir := t.TempDir()

	ideScript := filepath.Join(tmpDir, "idea.sh")
	err := os.WriteFile(ideScript, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	assert.NoError(t, err)

	configDir := filepath.Join(tmpDir, "config")
	assert.NoError(t, os.MkdirAll(configDir, 0o755))

	ctx := corescan.ContextBuilder{
		Analyser: product.JvmLinter.NativeAnalyzer(),
		Prod: product.Product{
			IdeScript:      ideScript,
			BaseScriptName: product.Idea,
			Code:           product.QDJVM,
			Build:          "242.1234",
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
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS == "windows" {
		t.Skip("test uses a shell script as a fake IDE")
	}

	tmpDir := t.TempDir()

	// Place the IDE script in a directory with spaces
	dirWithSpaces := filepath.Join(tmpDir, "My IDE")
	assert.NoError(t, os.MkdirAll(dirWithSpaces, 0o755))

	ideScript := filepath.Join(dirWithSpaces, "idea.sh")
	assert.NoError(t, os.WriteFile(ideScript, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	configDir := filepath.Join(tmpDir, "config")
	assert.NoError(t, os.MkdirAll(configDir, 0o755))

	ctx := corescan.ContextBuilder{
		Analyser: product.JvmLinter.NativeAnalyzer(),
		Prod: product.Product{
			IdeScript:      ideScript,
			BaseScriptName: product.Idea,
			Code:           product.QDJVM,
			Build:          "242.1234",
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

func TestRunQodanaLocal(t *testing.T) {
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS == "windows" {
		t.Skip("test uses a shell script as a fake IDE")
	}

	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	resultsDir := filepath.Join(tmpDir, "results")
	configDir := filepath.Join(tmpDir, "config")
	cacheDir := filepath.Join(tmpDir, "cache")

	for _, dir := range []string{projectDir, resultsDir, configDir, cacheDir} {
		assert.NoError(t, os.MkdirAll(dir, 0o755))
	}

	// Pre-create the short SARIF file that getIdeExitCode reads after the process finishes
	sarifContent := `{"runs": [{"invocations": [{"exitCode": 0}]}]}`
	assert.NoError(t, os.WriteFile(
		filepath.Join(resultsDir, "qodana-short.sarif.json"),
		[]byte(sarifContent), 0o644))

	// Create a fake IDE script that exits 0
	ideScript := filepath.Join(tmpDir, "idea.sh")
	assert.NoError(t, os.WriteFile(ideScript, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	ctx := corescan.ContextBuilder{
		Analyser: product.JvmLinter.NativeAnalyzer(),
		Prod: product.Product{
			IdeScript:      ideScript,
			BaseScriptName: product.Idea,
			Code:           product.QDJVM,
			Build:          "242.1234",
			Version:        "2024.2",
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
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS == "windows" {
		t.Skip("test uses a shell script as a fake IDE")
	}

	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "my project")
	resultsDir := filepath.Join(tmpDir, "my results")
	configDir := filepath.Join(tmpDir, "my config")
	cacheDir := filepath.Join(tmpDir, "my cache")

	for _, dir := range []string{projectDir, resultsDir, configDir, cacheDir} {
		assert.NoError(t, os.MkdirAll(dir, 0o755))
	}

	sarifContent := `{"runs": [{"invocations": [{"exitCode": 0}]}]}`
	assert.NoError(t, os.WriteFile(
		filepath.Join(resultsDir, "qodana-short.sarif.json"),
		[]byte(sarifContent), 0o644))

	dirWithSpaces := filepath.Join(tmpDir, "My IDE")
	assert.NoError(t, os.MkdirAll(dirWithSpaces, 0o755))
	ideScript := filepath.Join(dirWithSpaces, "idea.sh")
	assert.NoError(t, os.WriteFile(ideScript, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	ctx := corescan.ContextBuilder{
		Analyser: product.JvmLinter.NativeAnalyzer(),
		Prod: product.Product{
			IdeScript:      ideScript,
			BaseScriptName: product.Idea,
			Code:           product.QDJVM,
			Build:          "242.1234",
			Version:        "2024.2",
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
