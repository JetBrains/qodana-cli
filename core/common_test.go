package core

import (
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectAnalyzer(t *testing.T) {
	nativePathMaker := func(dir string) error {
		assetsPath := filepath.Join(dir, "Assets")
		projectSettingsPath := filepath.Join(dir, "ProjectSettings")
		_ = os.MkdirAll(assetsPath, os.ModePerm)
		_ = os.MkdirAll(projectSettingsPath, os.ModePerm)
		unityFile := filepath.Join(projectSettingsPath, "ProjectVersion.txt")
		_ = os.WriteFile(unityFile, []byte{}, os.ModePerm)
		return nil
	}
	nonNativePathMaker := func(dir string) error {
		return nil
	}

	tests := []struct {
		name             string
		pathMaker        func(string) error
		analyzers        []string
		interactive      bool
		selectFunc       func([]string) string
		expectedAnalyzer string
	}{
		{
			name:             "Empty Analyzers Non-interactive",
			pathMaker:        nonNativePathMaker,
			analyzers:        []string{},
			interactive:      false,
			selectFunc:       nil,
			expectedAnalyzer: "",
		},
		{
			name:             "Multiple Analyzers Non-interactive",
			pathMaker:        nonNativePathMaker,
			analyzers:        AllCodes,
			interactive:      false,
			selectFunc:       nil,
			expectedAnalyzer: Image(AllCodes[0]),
		},
		{
			name:             "Single .NET Analyzer Interactive Non Native",
			pathMaker:        nonNativePathMaker,
			analyzers:        []string{QDNET},
			interactive:      true,
			selectFunc:       func(choices []string) string { return choices[0] },
			expectedAnalyzer: Image(QDNET),
		},
		{
			name:             "Single .NET Analyzer Interactive Native",
			pathMaker:        nativePathMaker,
			analyzers:        []string{QDNET},
			interactive:      true,
			selectFunc:       func(choices []string) string { return choices[0] },
			expectedAnalyzer: QDNET,
		},
		{
			name:             "Single .NET Community Analyzer Interactive Native",
			pathMaker:        nativePathMaker,
			analyzers:        []string{QDNETC},
			interactive:      true,
			selectFunc:       func(choices []string) string { return choices[0] },
			expectedAnalyzer: Image(QDNETC),
		},
		{
			name:             "Multiple Analyzers Interactive",
			pathMaker:        nonNativePathMaker,
			analyzers:        AllCodes,
			interactive:      true,
			selectFunc:       func(choices []string) string { return choices[0] },
			expectedAnalyzer: Image(AllCodes[0]),
		},
		{
			name:             "Empty Choice Interactive",
			pathMaker:        nonNativePathMaker,
			analyzers:        AllCodes,
			interactive:      true,
			selectFunc:       func(choices []string) string { return "" },
			expectedAnalyzer: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "unity-project")
			if err != nil {
				t.Fatalf("Error creating tmp dir: %v", err)
			}
			defer func(path string) {
				err := os.RemoveAll(path)
				if err != nil {
					t.Fatalf("Error removing tmp dir: %v", err)
				}
			}(dir)
			_ = test.pathMaker(dir)
			got := SelectAnalyzer(dir, test.analyzers, test.interactive, test.selectFunc)
			assert.Equal(t, test.expectedAnalyzer, got)
		})
	}
}
