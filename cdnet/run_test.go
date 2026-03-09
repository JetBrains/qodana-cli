package main

import (
	"testing"

	"github.com/JetBrains/qodana-cli/internal/foundation/hash"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
)

func TestMountTools(t *testing.T) {
	testutil.Need(t, testutil.CdnetDeps)

	linter := CdnetLinter{}
	tempdir := t.TempDir()

	mountInfo, err := linter.MountTools(tempdir)
	if err != nil {
		t.Fatal(err)
	}

	path := mountInfo[thirdpartyscan.Clt]
	expectedHash := CltSha256
	actualHash, err := hash.GetFileSha256(path)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedHash, actualHash[:])
}
