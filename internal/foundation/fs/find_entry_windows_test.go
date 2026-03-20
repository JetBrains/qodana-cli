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

func TestFindEntry_CaseNormalization(t *testing.T) {
	dir := canonicalTempDir(t)
	touch(t, filepath.Join(dir, "MixedCase"))

	got, err := findEntry(dir, "mixedcase")
	require.NoError(t, err)
	assert.Equal(t, "MixedCase", got, "findEntry must return on-disk casing")
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

func TestFindEntry_FileInDirectory(t *testing.T) {
	dir := canonicalTempDir(t)
	touch(t, filepath.Join(dir, "TestFile.txt"))

	got, err := findEntry(dir, "testfile.txt")
	require.NoError(t, err)
	assert.Equal(t, "TestFile.txt", got, "findEntry must return on-disk casing")
}

func TestFindEntry_LongFilename(t *testing.T) {
	dir := canonicalTempDir(t)
	// 255-character name (NTFS component limit).
	name := strings.Repeat("a", 255)
	touch(t, filepath.Join(dir, name))

	got, err := findEntry(dir, name)
	require.NoError(t, err)
	assert.Equal(t, name, got)
}

func TestNormalizeCaseWindows_DriveLetterNormalized(t *testing.T) {
	dir := canonicalTempDir(t)
	// Construct a path with a lowercase drive letter.
	vol := filepath.VolumeName(dir)
	if vol == "" {
		t.Skip("no volume name on this platform")
	}
	lowered := strings.ToLower(vol) + dir[len(vol):]

	got, err := normalizeCaseWindows(lowered)
	require.NoError(t, err)
	assert.Equal(t, strings.ToUpper(vol), filepath.VolumeName(got),
		"drive letter must be uppercased")
}
