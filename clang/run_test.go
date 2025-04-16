package main

import (
	"os"
	"testing"

	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"github.com/stretchr/testify/assert"
)

func TestMountTools(t *testing.T) {
	// skip this test on GitHub due to missing artifacts
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip()
	}

	linter := ClangLinter{}
	tempdir := t.TempDir()

	mountInfo, err := linter.MountTools(tempdir)
	if err != nil {
		t.Fatal(err)
	}

	path := mountInfo[thirdpartyscan.Clang]
	expectedHash := ClangTidySha256
	actualHash, err := utils.GetFileSha256(path)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedHash, actualHash)
}
