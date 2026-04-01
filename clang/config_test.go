package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/stretchr/testify/assert"
)

func TestProcessConfig(t *testing.T) {
	tests := []struct {
		name           string
		includes       []qdyaml.Clude
		excludes       []qdyaml.Clude
		version        string
		expected       string
		hasClangTidy   bool
	}{
		{
			name:     "no configuration, no .clang-tidy - defaults to all checks",
			includes: nil,
			excludes: nil,
			version:  "",
			expected: "--checks=*",
		},
		{
			name:         "no configuration, has .clang-tidy - respects project config",
			includes:     nil,
			excludes:     nil,
			version:      "",
			expected:     "",
			hasClangTidy: true,
		},
		{
			name:         "has .clang-tidy with includes - includes take precedence",
			includes:     []qdyaml.Clude{{Name: "bugprone-*"}},
			excludes:     nil,
			version:      "1.0",
			expected:     "--checks=bugprone-*",
			hasClangTidy: true,
		},
		{
			name:     "only includes - enables specified checks",
			includes: []qdyaml.Clude{{Name: "bugprone-*"}},
			excludes: nil,
			version:  "1.0",
			expected: "--checks=bugprone-*",
		},
		{
			name:     "only excludes - enables all checks then excludes specified",
			includes: nil,
			excludes: []qdyaml.Clude{{Name: "CppDFAMemoryLeak"}},
			version:  "1.0",
			expected: "--checks=*,-CppDFAMemoryLeak",
		},
		{
			name:     "both includes and excludes - includes first then excludes",
			includes: []qdyaml.Clude{{Name: "bugprone-*"}},
			excludes: []qdyaml.Clude{{Name: "bugprone-argument-comment"}},
			version:  "1.0",
			expected: "--checks=bugprone-*,-bugprone-argument-comment",
		},
		{
			name:     "multiple excludes - all prefixed with minus",
			includes: nil,
			excludes: []qdyaml.Clude{
				{Name: "CppDFAMemoryLeak"},
				{Name: "CppDFAArrayIndexOutOfBounds"},
				{Name: "CppDFANullDereference"},
			},
			version:  "1.0",
			expected: "--checks=*,-CppDFAMemoryLeak,-CppDFAArrayIndexOutOfBounds,-CppDFANullDereference",
		},
		{
			name:     "multiple includes - joined with comma",
			includes: []qdyaml.Clude{{Name: "bugprone-*"}, {Name: "performance-*"}},
			excludes: nil,
			version:  "1.0",
			expected: "--checks=bugprone-*,performance-*",
		},
		{
			name:     "clion-prefixed includes are filtered out",
			includes: []qdyaml.Clude{{Name: "clion-misra-cpp2008-0-1-1"}},
			excludes: nil,
			version:  "1.0",
			expected: "--checks=*",
		},
		{
			name:     "clion-prefixed includes filtered - falls back to all checks with excludes",
			includes: []qdyaml.Clude{{Name: "clion-misra-cpp2008-0-1-1"}},
			excludes: []qdyaml.Clude{{Name: "CppDFAMemoryLeak"}},
			version:  "1.0",
			expected: "--checks=*,-CppDFAMemoryLeak",
		},
		{
			name:     "mixed valid and clion includes - only valid ones used",
			includes: []qdyaml.Clude{{Name: "clion-misra-cpp2008-0-1-1"}, {Name: "bugprone-*"}},
			excludes: nil,
			version:  "1.0",
			expected: "--checks=bugprone-*",
		},
		{
			name:     "includes with quotes are filtered out",
			includes: []qdyaml.Clude{{Name: `bugprone-"test"`}},
			excludes: nil,
			version:  "1.0",
			expected: "--checks=*",
		},
		{
			name:     "excludes with quotes are filtered out",
			includes: nil,
			excludes: []qdyaml.Clude{{Name: `CppDFA"test"`}},
			version:  "1.0",
			expected: "--checks=*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir := t.TempDir()
			if tt.hasClangTidy {
				err := os.WriteFile(filepath.Join(projectDir, ".clang-tidy"), []byte("Checks: '-*,bugprone-*'\n"), 0644)
				assert.NoError(t, err)
			}
			ctx := thirdpartyscan.ContextBuilder{
				ProjectDir: projectDir,
				QodanaYamlConfig: thirdpartyscan.QodanaYamlConfig{
					Version:  tt.version,
					Includes: tt.includes,
					Excludes: tt.excludes,
				},
			}.Build()

			result, err := processConfig(ctx)

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
