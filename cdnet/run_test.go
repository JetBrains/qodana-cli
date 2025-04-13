package main

import (
	"os"
	"testing"

	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"github.com/stretchr/testify/assert"
)

func TestMountTools(t *testing.T) {
	linter := CdnetLinter{}
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
	expectedHash := CltSha256
	actualHash, err := utils.GetFileSha256(path)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedHash, actualHash)
}
