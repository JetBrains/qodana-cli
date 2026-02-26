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

package corescan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/stretchr/testify/assert"
)

func TestContextBuilder_Build(t *testing.T) {
	builder := ContextBuilder{
		Id:                        "test-id",
		IdeDir:                    "/opt/ide",
		EffectiveConfigurationDir: "/config",
		GlobalConfigurationsDir:   "/global",
		GlobalConfigurationId:     "global-id",
		CustomLocalQodanaYamlPath: "custom.yaml",
		QodanaYamlConfig: QodanaYamlConfig{
			Bootstrap: "echo test",
			Plugins:   []qdyaml.Plugin{{Id: "plugin1"}},
		},
		Prod:                      product.Product{Code: "QDJVM"},
		QodanaUploadToken:         "token123",
		ProjectDir:                "/project",
		RepositoryRoot:            "/repo",
		ResultsDir:                "/results",
		ConfigDir:                 "/configdir",
		LogDir:                    "/logs",
		QodanaSystemDir:           "/system",
		CacheDir:                  "/cache",
		ReportDir:                 "/report",
		CoverageDir:               "/coverage",
		OnlyDirectory:             "./src",
		Env:                       []string{"FOO=bar"},
		DisableSanity:             true,
		ProfileName:               "Default",
		ProfilePath:               "/profile.xml",
		RunPromo:                  "true",
		Baseline:                  "baseline.sarif",
		BaselineIncludeAbsent:     true,
		SaveReport:                true,
		ShowReport:                true,
		ShowReportPort:            8080,
		Property:                  []string{"prop=val"},
		Script:                    "default",
		FailThreshold:             "10",
		Commit:                    "abc123",
		DiffStart:                 "",
		DiffEnd:                   "def456",
		ForceLocalChangesScript:   false,
		ReversePrAnalysis:         false,
		AnalysisId:                "analysis-1",
		Volumes:                   []string{"/vol1:/vol1"},
		User:                      "1000:1000",
		PrintProblems:             true,
		GenerateCodeClimateReport: true,
		SendBitBucketInsights:     true,
		SkipPull:                  true,
		FullHistory:               false,
		ApplyFixes:                true,
		Cleanup:                   false,
		FixesStrategy:             "apply",
		NoStatistics:              true,
		CdnetSolution:             "solution.sln",
		CdnetProject:              "project.csproj",
		CdnetConfiguration:        "Release",
		CdnetPlatform:             "x64",
		CdnetNoBuild:              true,
		ClangCompileCommands:      "compile_commands.json",
		ClangArgs:                 "-Wall",
		AnalysisTimeoutMs:         60000,
		AnalysisTimeoutExitCode:   2,
		JvmDebugPort:              5005,
	}

	ctx := builder.Build()

	assert.Equal(t, "test-id", ctx.Id())
	assert.Equal(t, "/opt/ide", ctx.IdeDir())
	assert.Equal(t, "/config", ctx.EffectiveConfigurationDir())
	assert.Equal(t, "/global", ctx.GlobalConfigurationsDir())
	assert.Equal(t, "global-id", ctx.GlobalConfigurationId())
	assert.Equal(t, "custom.yaml", ctx.CustomLocalQodanaYamlPath())
	assert.Equal(t, "echo test", ctx.QodanaYamlConfig().Bootstrap)
	assert.Equal(t, "QDJVM", ctx.Prod().Code)
	assert.Equal(t, "token123", ctx.QodanaUploadToken())
	assert.Equal(t, "/project", ctx.ProjectDir())
	assert.Equal(t, "/repo", ctx.RepositoryRoot())
	assert.Equal(t, "/results", ctx.ResultsDir())
	assert.Equal(t, "/configdir", ctx.ConfigDir())
	assert.Equal(t, "/logs", ctx.LogDir())
	assert.Equal(t, "/system", ctx.QodanaSystemDir())
	assert.Equal(t, "/cache", ctx.CacheDir())
	assert.Equal(t, "/report", ctx.ReportDir())
	assert.Equal(t, "/coverage", ctx.CoverageDir())
	assert.Equal(t, "./src", ctx.OnlyDirectory())
	assert.True(t, ctx.DisableSanity())
	assert.Equal(t, "Default", ctx.ProfileName())
	assert.Equal(t, "/profile.xml", ctx.ProfilePath())
	assert.Equal(t, "true", ctx.RunPromo())
	assert.Equal(t, "baseline.sarif", ctx.Baseline())
	assert.True(t, ctx.BaselineIncludeAbsent())
	assert.True(t, ctx.SaveReport())
	assert.True(t, ctx.ShowReport())
	assert.Equal(t, 8080, ctx.ShowReportPort())
	assert.Equal(t, "default", ctx.Script())
	assert.Equal(t, "10", ctx.FailThreshold())
	assert.Equal(t, "abc123", ctx.Commit())
	assert.Equal(t, "", ctx.DiffStart())
	assert.Equal(t, "def456", ctx.DiffEnd())
	assert.False(t, ctx.ForceLocalChangesScript())
	assert.False(t, ctx.ReversePrAnalysis())
	assert.Equal(t, "", ctx.ReducedScopePath())
	assert.Equal(t, "analysis-1", ctx.AnalysisId())
	assert.Equal(t, "1000:1000", ctx.User())
	assert.True(t, ctx.PrintProblems())
	assert.True(t, ctx.GenerateCodeClimateReport())
	assert.True(t, ctx.SendBitBucketInsights())
	assert.True(t, ctx.SkipPull())
	assert.False(t, ctx.FullHistory())
	assert.True(t, ctx.ApplyFixes())
	assert.False(t, ctx.Cleanup())
	assert.Equal(t, "apply", ctx.FixesStrategy())
	assert.True(t, ctx.NoStatistics())
	assert.Equal(t, "solution.sln", ctx.CdnetSolution())
	assert.Equal(t, "project.csproj", ctx.CdnetProject())
	assert.Equal(t, "Release", ctx.CdnetConfiguration())
	assert.Equal(t, "x64", ctx.CdnetPlatform())
	assert.True(t, ctx.CdnetNoBuild())
	assert.Equal(t, "compile_commands.json", ctx.ClangCompileCommands())
	assert.Equal(t, "-Wall", ctx.ClangArgs())
	assert.Equal(t, 60000, ctx.AnalysisTimeoutMs())
	assert.Equal(t, 2, ctx.AnalysisTimeoutExitCode())
	assert.Equal(t, 5005, ctx.JvmDebugPort())

	env := ctx.Env()
	assert.Equal(t, []string{"FOO=bar"}, env)

	property := ctx.Property()
	assert.Equal(t, []string{"prop=val"}, property)

	volumes := ctx.Volumes()
	assert.Equal(t, []string{"/vol1:/vol1"}, volumes)
}

func TestContext_StartHash(t *testing.T) {
	tests := []struct {
		name      string
		commit    string
		diffStart string
		wantHash  string
		wantErr   bool
	}{
		{
			name:      "commit equals diffStart",
			commit:    "abc123",
			diffStart: "abc123",
			wantHash:  "abc123",
			wantErr:   false,
		},
		{
			name:      "commit empty, diffStart set",
			commit:    "",
			diffStart: "def456",
			wantHash:  "def456",
			wantErr:   false,
		},
		{
			name:      "commit set, diffStart empty",
			commit:    "abc123",
			diffStart: "",
			wantHash:  "abc123",
			wantErr:   false,
		},
		{
			name:      "both empty",
			commit:    "",
			diffStart: "",
			wantHash:  "",
			wantErr:   false,
		},
		{
			name:      "conflicting values",
			commit:    "abc123",
			diffStart: "def456",
			wantHash:  "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := ContextBuilder{
				Commit:    tt.commit,
				DiffStart: tt.diffStart,
			}.Build()

			hash, err := ctx.StartHash()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantHash, hash)
			}
		})
	}
}

func TestContext_DetermineRunScenario(t *testing.T) {
	tests := []struct {
		name         string
		fullHistory  bool
		hasStartHash bool
		forceLocal   bool
		isContainer  bool
		reversePr    bool
		expected     RunScenario
	}{
		{
			name:         "full history mode",
			fullHistory:  true,
			hasStartHash: true,
			expected:     RunScenarioFullHistory,
		},
		{
			name:         "no start hash",
			fullHistory:  false,
			hasStartHash: false,
			expected:     RunScenarioDefault,
		},
		{
			name:         "force local changes",
			fullHistory:  false,
			hasStartHash: true,
			forceLocal:   true,
			expected:     RunScenarioLocalChanges,
		},
		{
			name:         "container mode",
			fullHistory:  false,
			hasStartHash: true,
			forceLocal:   false,
			isContainer:  true,
			expected:     RunScenarioDefault,
		},
		{
			name:         "reverse PR analysis",
			fullHistory:  false,
			hasStartHash: true,
			forceLocal:   false,
			isContainer:  false,
			reversePr:    true,
			expected:     RunScenarioReversedScoped,
		},
		{
			name:         "scoped scenario",
			fullHistory:  false,
			hasStartHash: true,
			forceLocal:   false,
			isContainer:  false,
			reversePr:    false,
			expected:     RunScenarioScoped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var analyser product.Analyzer
			if tt.isContainer {
				analyser = product.JvmLinter.DockerAnalyzer()
			} else {
				analyser = product.JvmLinter.NativeAnalyzer()
			}

			ctx := ContextBuilder{
				FullHistory:             tt.fullHistory,
				ForceLocalChangesScript: tt.forceLocal,
				ReversePrAnalysis:       tt.reversePr,
				Analyser:                analyser,
			}.Build()

			result := ctx.DetermineRunScenario(tt.hasStartHash)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContext_VmOptionsPath(t *testing.T) {
	ctx := ContextBuilder{
		ConfigDir: "/config",
	}.Build()

	expected := filepath.Join("/config", "ide.vmoptions")
	assert.Equal(t, expected, ctx.VmOptionsPath())
}

func TestContext_InstallPluginsVmOptionsPath(t *testing.T) {
	ctx := ContextBuilder{
		ConfigDir: "/config",
	}.Build()

	expected := filepath.Join("/config", "install_plugins.vmoptions")
	assert.Equal(t, expected, ctx.InstallPluginsVmOptionsPath())
}

func TestContext_PropertiesAndFlags(t *testing.T) {
	ctx := ContextBuilder{
		Property: []string{
			"key1=value1",
			"key2=value2",
			"-flag1",
			"-flag2",
			"key3=value=with=equals",
		},
	}.Build()

	props, flags := ctx.PropertiesAndFlags()

	assert.Equal(t, "value1", props["key1"])
	assert.Equal(t, "value2", props["key2"])
	assert.Equal(t, "value=with=equals", props["key3"])
	assert.Contains(t, flags, "-flag1")
	assert.Contains(t, flags, "-flag2")
	assert.Len(t, flags, 2)
}

func TestContext_GetAnalysisTimeout(t *testing.T) {
	tests := []struct {
		name      string
		timeoutMs int
		expected  time.Duration
	}{
		{
			name:      "positive timeout",
			timeoutMs: 60000,
			expected:  60000 * time.Millisecond,
		},
		{
			name:      "zero timeout returns max",
			timeoutMs: 0,
			expected:  time.Duration(1<<63 - 1), // math.MaxInt64
		},
		{
			name:      "negative timeout returns max",
			timeoutMs: -1,
			expected:  time.Duration(1<<63 - 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := ContextBuilder{
				AnalysisTimeoutMs: tt.timeoutMs,
			}.Build()

			result := ctx.GetAnalysisTimeout()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContext_LocalQodanaYamlExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qodana-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	t.Run("yaml exists", func(t *testing.T) {
		yamlPath := filepath.Join(tmpDir, "qodana.yaml")
		err := os.WriteFile(yamlPath, []byte("version: \"1.0\""), 0o644)
		if err != nil {
			t.Fatal(err)
		}

		ctx := ContextBuilder{
			ProjectDir: tmpDir,
		}.Build()

		assert.True(t, ctx.LocalQodanaYamlExists())
	})

	t.Run("yaml does not exist", func(t *testing.T) {
		emptyDir, err := os.MkdirTemp("", "qodana-empty")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = os.RemoveAll(emptyDir)
		}()

		ctx := ContextBuilder{
			ProjectDir: emptyDir,
		}.Build()

		assert.False(t, ctx.LocalQodanaYamlExists())
	})
}

func TestContext_ProjectDirPathRelativeToRepositoryRoot(t *testing.T) {
	tests := []struct {
		name           string
		projectDir     string
		repositoryRoot string
		expected       string
	}{
		{
			name:           "same directory",
			projectDir:     "/repo",
			repositoryRoot: "/repo",
			expected:       ".",
		},
		{
			name:           "subdirectory",
			projectDir:     "/repo/subproject",
			repositoryRoot: "/repo",
			expected:       "subproject",
		},
		{
			name:           "nested subdirectory",
			projectDir:     "/repo/sub1/sub2",
			repositoryRoot: "/repo",
			expected:       "sub1/sub2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := ContextBuilder{
				ProjectDir:     tt.projectDir,
				RepositoryRoot: tt.repositoryRoot,
			}.Build()

			result := ctx.ProjectDirPathRelativeToRepositoryRoot()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsScopedScenario(t *testing.T) {
	tests := []struct {
		scenario string
		expected bool
	}{
		{RunScenarioScoped, true},
		{RunScenarioReversedScoped, true},
		{RunScenarioDefault, false},
		{RunScenarioFullHistory, false},
		{RunScenarioLocalChanges, false},
	}

	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			result := IsScopedScenario(tt.scenario)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestYamlConfig(t *testing.T) {
	yaml := qdyaml.QodanaYaml{
		Bootstrap: "echo test",
		Plugins:   []qdyaml.Plugin{{Id: "plugin1"}, {Id: "plugin2"}},
		Properties: map[string]string{
			"key1": "value1",
		},
		DotNet: qdyaml.DotNet{
			Solution: "test.sln",
		},
	}

	config := YamlConfig(yaml)

	assert.Equal(t, "echo test", config.Bootstrap)
	assert.Len(t, config.Plugins, 2)
	assert.Equal(t, "plugin1", config.Plugins[0].Id)
	assert.Equal(t, "value1", config.Properties["key1"])
	assert.Equal(t, "test.sln", config.DotNet.Solution)
}

func TestArrayCopy(t *testing.T) {
	original := []string{"a", "b", "c"}
	copied := arrayCopy(original)
	assert.Equal(t, original, copied)
	copied[0] = "modified"
	assert.Equal(t, "a", original[0])
	assert.Equal(t, "modified", copied[0])
}
