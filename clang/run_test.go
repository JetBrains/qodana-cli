package main

import (
	"os"
	"testing"

	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"
	"github.com/stretchr/testify/assert"
)

func TestMountTools(t *testing.T) {
	linter := ClangLinter{}
	tempdir, err := os.MkdirTemp("", "TestMountTools")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = os.RemoveAll(tempdir)
		if err != nil {
			t.Fatal(err)
		}
	}()

	mountInfo, err := linter.MountTools(tempdir)
	if err != nil {
		t.Fatal(err)
	}

	path := mountInfo[thirdpartyscan.Clang]
	expectedHash := ClangTidySha256
	actualHash, err := getSha256(path)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedHash, actualHash)
}
