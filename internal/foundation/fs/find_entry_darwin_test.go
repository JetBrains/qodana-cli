//go:build darwin

package fs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindEntry_ExactMatch(t *testing.T) {
	dir := canonicalTempDir(t)
	touch(t, filepath.Join(dir, "Hello"))

	got, err := findEntry(dir, "Hello")
	require.NoError(t, err)
	assert.Equal(t, "Hello", got)
}

func TestFindEntry_NotFound(t *testing.T) {
	dir := canonicalTempDir(t)

	_, err := findEntry(dir, "nonexistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestFindEntry_NotFoundParentMissing(t *testing.T) {
	dir := canonicalTempDir(t)

	_, err := findEntry(filepath.Join(dir, "no-such-parent"), "child")
	require.Error(t, err)
}

func TestFindEntry_CaseNormalization(t *testing.T) {
	skipIfCaseSensitive(t)
	dir := canonicalTempDir(t)
	touch(t, filepath.Join(dir, "MixedCase"))

	got, err := findEntry(dir, "mixedcase")
	require.NoError(t, err)
	assert.Equal(t, "MixedCase", got, "findEntry must return on-disk casing")
}

func TestFindEntry_LongFilename(t *testing.T) {
	dir := canonicalTempDir(t)
	// 255-byte ASCII name (NAME_MAX on macOS).
	name := strings.Repeat("a", 255)
	touch(t, filepath.Join(dir, name))

	got, err := findEntry(dir, name)
	require.NoError(t, err)
	assert.Equal(t, name, got)
}

func TestFindEntry_LongFilenameMultibyteUTF8(t *testing.T) {
	dir := canonicalTempDir(t)
	// Each CJK character is 3 bytes in UTF-8. 100 characters = 300 bytes,
	// which exceeds the old 256-byte buffer but is well within NAME_MAX
	// (255 characters on APFS).
	name := strings.Repeat("漢", 100)
	touch(t, filepath.Join(dir, name))

	got, err := findEntry(dir, name)
	require.NoError(t, err)
	assert.Equal(t, name, got)
}
