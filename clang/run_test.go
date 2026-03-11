package main

import (
	"testing"

	"github.com/JetBrains/qodana-cli/internal/foundation/hash"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/internal/testutil/needs"
	"github.com/stretchr/testify/assert"
)

func TestMountTools(t *testing.T) {
	needs.Need(t, needs.ClangDeps)

	linter := ClangLinter{}
	tempdir := t.TempDir()

	mountInfo, err := linter.MountTools(tempdir)
	if err != nil {
		t.Fatal(err)
	}

	path := mountInfo[thirdpartyscan.Clang]
	expectedHash := ClangTidySha256
	actualHash, err := hash.GetFileSha256(path)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedHash, actualHash[:])
}
