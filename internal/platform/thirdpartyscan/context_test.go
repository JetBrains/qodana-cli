package thirdpartyscan

import (
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/stretchr/testify/assert"
)

func TestContextBuilder_Build(t *testing.T) {
	builder := ContextBuilder{
		LinterInfo: LinterInfo{
			ProductCode:           "QDCLC",
			LinterPresentableName: "Qodana Community for C/C++",
			LinterName:            "qodana-clang",
			LinterVersion:         "2024.1",
			IsEap:                 false,
		},
		MountInfo: MountInfo{
			CustomTools: map[string]string{Clang: "/usr/bin/clang"},
		},
		CloudData: ThirdPartyStartupCloudData{
			LicensePlan:   "COMMUNITY",
			ProjectIdHash: "hash123",
			QodanaToken:   "token123",
		},
		ProjectDir:                "/project",
		ResultsDir:                "/results",
		ReportDir:                 "/report",
		LogDir:                    "/logs",
		CacheDir:                  "/cache",
		ClangCompileCommands:      "compile_commands.json",
		ClangArgs:                 "-Wall",
		Property:                  []string{"prop=val"},
		CdnetSolution:             "solution.sln",
		CdnetProject:              "project.csproj",
		CdnetConfiguration:        "Release",
		CdnetPlatform:             "x64",
		NoStatistics:              true,
		CdnetNoBuild:              true,
		AnalysisId:                "analysis-1",
		Baseline:                  "baseline.sarif",
		BaselineIncludeAbsent:     true,
		FailThreshold:             "10",
		GenerateCodeClimateReport: true,
		SendBitBucketInsights:     true,
		QodanaYamlConfig: QodanaYamlConfig{
			Bootstrap: "echo test",
			Version:   "1.0",
		},
	}

	ctx := builder.Build()

	assert.Equal(t, "QDCLC", ctx.LinterInfo().ProductCode)
	assert.Equal(t, "COMMUNITY", ctx.CloudData().LicensePlan)
	assert.Equal(t, "/project", ctx.ProjectDir())
	assert.Equal(t, "/results", ctx.ResultsDir())
	assert.Equal(t, "/report", ctx.ReportDir())
	assert.Equal(t, "/logs", ctx.LogDir())
	assert.Equal(t, "/cache", ctx.CacheDir())
	assert.Equal(t, "compile_commands.json", ctx.ClangCompileCommands())
	assert.Equal(t, "-Wall", ctx.ClangArgs())
	assert.Equal(t, []string{"prop=val"}, ctx.Property())
	assert.Equal(t, "solution.sln", ctx.CdnetSolution())
	assert.Equal(t, "project.csproj", ctx.CdnetProject())
	assert.Equal(t, "Release", ctx.CdnetConfiguration())
	assert.Equal(t, "x64", ctx.CdnetPlatform())
	assert.True(t, ctx.NoStatistics())
	assert.True(t, ctx.CdnetNoBuild())
	assert.Equal(t, "analysis-1", ctx.AnalysisId())
	assert.Equal(t, "baseline.sarif", ctx.Baseline())
	assert.True(t, ctx.BaselineIncludeAbsent())
	assert.Equal(t, "10", ctx.FailThreshold())
	assert.True(t, ctx.GenerateCodeClimateReport())
	assert.True(t, ctx.SendBitBucketInsights())
	assert.Equal(t, "echo test", ctx.QodanaYamlConfig().Bootstrap)
}

func TestContext_IsCommunity(t *testing.T) {
	community := ContextBuilder{CloudData: ThirdPartyStartupCloudData{LicensePlan: "COMMUNITY"}}.Build()
	assert.True(t, community.IsCommunity())

	paid := ContextBuilder{CloudData: ThirdPartyStartupCloudData{LicensePlan: "ULTIMATE"}}.Build()
	assert.False(t, paid.IsCommunity())
}

func TestContext_ClangPath(t *testing.T) {
	ctx := ContextBuilder{
		MountInfo: MountInfo{CustomTools: map[string]string{Clang: "/usr/bin/clang-15"}},
	}.Build()
	assert.Equal(t, "/usr/bin/clang-15", ctx.ClangPath())
}

func TestContext_Property(t *testing.T) {
	ctx := ContextBuilder{Property: []string{"a=1", "b=2"}}.Build()
	props := ctx.Property()
	assert.Equal(t, []string{"a=1", "b=2"}, props)
	props[0] = "modified"
	assert.Equal(t, "a=1", ctx.Property()[0])
}

func TestLinterInfo_GetMajorVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected string
	}{
		{"2024.1", "2024.1"},
		{"2024.1.5", "2024.1"},
		{"2024.1-eap", "2024.1"},
		{"invalid", "2025.3"},
		{"", "2025.3"},
	}
	for _, tt := range tests {
		t.Run(
			tt.version, func(t *testing.T) {
				info := LinterInfo{LinterVersion: tt.version}
				assert.Equal(t, tt.expected, info.GetMajorVersion())
			},
		)
	}
}

func TestYamlConfig(t *testing.T) {
	threshold := 5
	yaml := qdyaml.QodanaYaml{
		Bootstrap:     "echo test",
		Version:       "1.0",
		FailThreshold: &threshold,
		DotNet:        qdyaml.DotNet{Solution: "test.sln"},
	}
	config := YamlConfig(yaml)
	assert.Equal(t, "echo test", config.Bootstrap)
	assert.Equal(t, "1.0", config.Version)
	assert.Equal(t, 5, *config.FailThreshold)
	assert.Equal(t, "test.sln", config.DotNet.Solution)
}
