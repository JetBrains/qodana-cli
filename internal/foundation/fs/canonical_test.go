package fs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helpers =============================================================

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	result, err := Canonical(dir)
	require.NoError(t, err, "failed to canonicalize temp dir %q", dir)
	return result
}

func mkdirp(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, 0o755))
}

func touch(t *testing.T, path string) {
	t.Helper()
	mkdirp(t, filepath.Dir(path))
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func makeSymlink(t *testing.T, target, linkPath string) {
	t.Helper()
	require.NoError(t, os.Symlink(target, linkPath))
}

func isTestDirCaseSensitive(t *testing.T) bool {
	t.Helper()
	dir := t.TempDir()
	mkdirp(t, filepath.Join(dir, "A"))
	_, err := os.Stat(filepath.Join(dir, "a"))
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	require.NoError(t, err)
	return false
}

func skipIfCaseSensitive(t *testing.T) {
	t.Helper()
	if isTestDirCaseSensitive(t) {
		t.Skip("not relevant on case-sensitive filesystems")
	}
}

func skipIfCaseInsensitive(t *testing.T) {
	t.Helper()
	if !isTestDirCaseSensitive(t) {
		t.Skip("not relevant on case-insensitive filesystems")
	}
}

// Category 1: Input validation =============================================

func TestCanonical_NonExistentPath(t *testing.T) {
	tmp := canonicalTempDir(t)
	_, err := Canonical(filepath.Join(tmp, "missing"))
	assert.True(t, errors.Is(err, os.ErrNotExist), "expected IsNotExist, got: %v", err)
}

func TestCanonical_NonExistentIntermediateDir(t *testing.T) {
	tmp := canonicalTempDir(t)
	touch(t, filepath.Join(tmp, "file"))
	_, err := Canonical(filepath.Join(tmp, "missing", "file"))
	assert.Error(t, err)
}

func TestCanonical_EmptyString(t *testing.T) {
	_, err := Canonical("")
	assert.Error(t, err)
}

// Category 2: Path normalization ===========================================

func TestCanonical_Noop(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "file")
	touch(t, expected)

	actual, err := Canonical(expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_Dot(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "dir", "file")
	touch(t, expected)

	actual, err := Canonical(filepath.Join(tmp, "dir", ".", "file"))
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_TrailingDot(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "dir")
	mkdirp(t, expected)

	actual, err := Canonical(expected + string(os.PathSeparator) + ".")
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_DotDot(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "dir")
	mkdirp(t, expected)

	actual, err := Canonical(filepath.Join(tmp, "dir", "..", "dir"))
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_DotDotAboveRoot(t *testing.T) {
	var root string
	if runtime.GOOS == "windows" {
		root = filepath.VolumeName(os.TempDir()) + `\`
	} else {
		root = "/"
	}

	actual, err := Canonical(filepath.Join(root, "..", "..", ".."))
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(root), actual)
}

func TestCanonical_MultipleSlashes(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "file")
	touch(t, expected)

	actual, err := Canonical(tmp + string(os.PathSeparator) + string(os.PathSeparator) + string(os.PathSeparator) + string(os.PathSeparator) + "file")
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_TrailingSlash(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "dir")
	mkdirp(t, expected)

	actual, err := Canonical(expected + string(os.PathSeparator))
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_MultipleDots(t *testing.T) {
	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "dir"))

	actual, err := Canonical(filepath.Join(tmp, ".", ".", ".", "dir"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "dir"), actual)
}

func TestCanonical_NonExistentIntermediateDotDot(t *testing.T) {
	// glibc test #5: /existing/nonexistent/../existing must fail because
	// nonexistent is traversed before .. is applied.
	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "existing"))

	_, err := Canonical(filepath.Join(tmp, "existing", "nonexistent", "..", "existing"))
	assert.Error(t, err)
}

// Category 3: Relative paths ===============================================

func TestCanonical_RelativePath(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "dir")
	mkdirp(t, expected)

	t.Chdir(tmp)
	actual, err := Canonical("dir")
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_RelativeDot(t *testing.T) {
	tmp := canonicalTempDir(t)
	t.Chdir(tmp)

	actual, err := Canonical(".")
	require.NoError(t, err)
	assert.Equal(t, tmp, actual)
}

func TestCanonical_RelativeDotDot(t *testing.T) {
	tmp := canonicalTempDir(t)
	child := filepath.Join(tmp, "child")
	mkdirp(t, child)

	t.Chdir(child)
	actual, err := Canonical("..")
	require.NoError(t, err)
	assert.Equal(t, tmp, actual)
}

// Category 4: Symlink resolution ===========================================

func TestCanonical_TrailingSymlink(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "dir")
	mkdirp(t, expected)
	makeSymlink(t, "dir", filepath.Join(tmp, "link"))

	actual, err := Canonical(filepath.Join(tmp, "link"))
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_IntermediateSymlink(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "dir", "nested")
	mkdirp(t, expected)
	makeSymlink(t, "dir", filepath.Join(tmp, "link"))

	actual, err := Canonical(filepath.Join(tmp, "link", "nested"))
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_SymlinkChain(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "real")
	mkdirp(t, expected)
	makeSymlink(t, "real", filepath.Join(tmp, "link3"))
	makeSymlink(t, "link3", filepath.Join(tmp, "link2"))
	makeSymlink(t, "link2", filepath.Join(tmp, "link1"))

	actual, err := Canonical(filepath.Join(tmp, "link1"))
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_RelativeSymlinkTarget(t *testing.T) {
	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "a", "b"))
	mkdirp(t, filepath.Join(tmp, "a", "other"))
	// link target is relative: ../other
	makeSymlink(t, filepath.Join("..", "other"), filepath.Join(tmp, "a", "b", "link"))

	actual, err := Canonical(filepath.Join(tmp, "a", "b", "link"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "a", "other"), actual)
}

func TestCanonical_AbsoluteSymlinkTarget(t *testing.T) {
	tmp := canonicalTempDir(t)
	target := filepath.Join(tmp, "target")
	mkdirp(t, target)
	makeSymlink(t, target, filepath.Join(tmp, "link"))

	actual, err := Canonical(filepath.Join(tmp, "link"))
	require.NoError(t, err)
	assert.Equal(t, target, actual)
}

func TestCanonical_SymlinkToFile(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "file")
	touch(t, expected)
	makeSymlink(t, "file", filepath.Join(tmp, "link"))

	actual, err := Canonical(filepath.Join(tmp, "link"))
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_SymlinkToDir(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "dir")
	mkdirp(t, expected)
	makeSymlink(t, "dir", filepath.Join(tmp, "link"))

	actual, err := Canonical(filepath.Join(tmp, "link"))
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_SymlinkThenDotDot(t *testing.T) {
	// From PR #584: symlink/.. should resolve .. relative to the symlink's
	// target, not relative to the symlink's location.
	// Setup:  1/1.1 (dir), 2 (dir), 2/link -> 1/1.1
	// Query:  2/link/..  should yield  1  (parent of 1/1.1), NOT  2.
	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "1", "1.1"))
	mkdirp(t, filepath.Join(tmp, "2"))
	makeSymlink(t, filepath.Join(tmp, "1", "1.1"), filepath.Join(tmp, "2", "link"))

	expected := filepath.Join(tmp, "1")
	// Do not use filepath.Join for ".." — it would collapse "link/.." lexically.
	query := filepath.Join(tmp, "2", "link") + string(os.PathSeparator) + ".."
	actual, err := Canonical(query)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_RelativeSymlinkDotDot(t *testing.T) {
	// Same as SymlinkThenDotDot but with relative input after Chdir.
	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "1", "1.1"))
	mkdirp(t, filepath.Join(tmp, "2"))
	makeSymlink(t, filepath.Join(tmp, "1", "1.1"), filepath.Join(tmp, "2", "link"))

	expected := filepath.Join(tmp, "1")
	t.Chdir(tmp)
	// Do not use filepath.Join for ".." — it would collapse "link/.." lexically.
	query := filepath.Join("2", "link") + string(os.PathSeparator) + ".."
	actual, err := Canonical(query)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_NestedSymlinkWithDotDot(t *testing.T) {
	// gnulib regression: nested symlinks + .. that previously caused false ELOOP.
	// Setup:  dir/subdir (real), link1 -> dir, link2 -> link1/subdir
	// Query:  link2/..  should yield  dir  (not ELOOP)
	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "dir", "subdir"))
	makeSymlink(t, "dir", filepath.Join(tmp, "link1"))
	// Note: "link1/subdir" as a symlink target is fine — it's not passed through filepath.Join's Clean.
	makeSymlink(t, "link1"+string(os.PathSeparator)+"subdir", filepath.Join(tmp, "link2"))

	expected := filepath.Join(tmp, "dir")
	// Do not use filepath.Join for ".." — it would collapse "link2/.." lexically.
	query := filepath.Join(tmp, "link2") + string(os.PathSeparator) + ".."
	actual, err := Canonical(query)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

// Category 5: Broken and circular symlinks =================================

func TestCanonical_BrokenSymlink(t *testing.T) {
	tmp := canonicalTempDir(t)
	makeSymlink(t, "nonexistent", filepath.Join(tmp, "broken"))

	_, err := Canonical(filepath.Join(tmp, "broken"))
	assert.Error(t, err)
}

func TestCanonical_CircularSymlink(t *testing.T) {
	tmp := canonicalTempDir(t)
	makeSymlink(t, "b", filepath.Join(tmp, "a"))
	makeSymlink(t, "a", filepath.Join(tmp, "b"))

	_, err := Canonical(filepath.Join(tmp, "a"))
	assert.Error(t, err)
}

func TestCanonical_SelfReferentialSymlink(t *testing.T) {
	tmp := canonicalTempDir(t)
	makeSymlink(t, "self", filepath.Join(tmp, "self"))

	_, err := Canonical(filepath.Join(tmp, "self"))
	assert.Error(t, err)
}

func TestCanonical_DeepSymlinkChain(t *testing.T) {
	// 10 levels of valid symlinks — must still resolve (not false ELOOP).
	tmp := canonicalTempDir(t)
	target := filepath.Join(tmp, "real")
	mkdirp(t, target)

	prev := "real"
	for i := range 10 {
		name := filepath.Join(tmp, "link"+string(rune('a'+i)))
		makeSymlink(t, prev, name)
		prev = filepath.Base(name)
	}

	actual, err := Canonical(filepath.Join(tmp, prev))
	require.NoError(t, err)
	assert.Equal(t, target, actual)
}

func TestWeaklyCanonical_CircularSymlink(t *testing.T) {
	tmp := canonicalTempDir(t)
	makeSymlink(t, "b", filepath.Join(tmp, "a"))
	makeSymlink(t, "a", filepath.Join(tmp, "b"))

	_, err := WeaklyCanonical(filepath.Join(tmp, "a"))
	assert.Error(t, err, "circular symlink must return error, not infinite loop")
}

func TestWeaklyCanonical_SelfReferentialSymlink(t *testing.T) {
	tmp := canonicalTempDir(t)
	makeSymlink(t, "self", filepath.Join(tmp, "self"))

	_, err := WeaklyCanonical(filepath.Join(tmp, "self"))
	assert.Error(t, err, "self-referential symlink must return error, not infinite loop")
}

func TestWeaklyCanonical_CircularSymlinkWithTail(t *testing.T) {
	tmp := canonicalTempDir(t)
	makeSymlink(t, "b", filepath.Join(tmp, "a"))
	makeSymlink(t, "a", filepath.Join(tmp, "b"))

	_, err := WeaklyCanonical(filepath.Join(tmp, "a", "child"))
	assert.Error(t, err, "circular symlink with tail must return error, not infinite loop")
}

// Category 6: Non-directory in path (ENOTDIR) ==============================

func TestCanonical_FileAsDirectory(t *testing.T) {
	tmp := canonicalTempDir(t)
	touch(t, filepath.Join(tmp, "file"))

	_, err := Canonical(filepath.Join(tmp, "file", "child"))
	assert.Error(t, err)
}

func TestCanonical_FileWithTrailingSlash(t *testing.T) {
	tmp := canonicalTempDir(t)
	touch(t, filepath.Join(tmp, "file"))

	_, err := Canonical(filepath.Join(tmp, "file") + string(os.PathSeparator))
	assert.Error(t, err)
}

func TestCanonical_SymlinkToFileWithTrailingPath(t *testing.T) {
	tmp := canonicalTempDir(t)
	touch(t, filepath.Join(tmp, "file"))
	makeSymlink(t, "file", filepath.Join(tmp, "link"))

	_, err := Canonical(filepath.Join(tmp, "link", "child"))
	assert.Error(t, err)
}

// Category 7: Case normalization ===========================================

func TestCanonical_CaseInsensitive(t *testing.T) {
	skipIfCaseSensitive(t)

	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "Aa")
	touch(t, expected)

	actual, err := Canonical(filepath.Join(tmp, "aA"))
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_CaseInsensitiveSymlinkTarget(t *testing.T) {
	skipIfCaseSensitive(t)

	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "dir")
	mkdirp(t, expected)
	// Symlink target uses wrong case
	makeSymlink(t, "DIR", filepath.Join(tmp, "link"))

	actual, err := Canonical(filepath.Join(tmp, "link"))
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_CaseInsensitiveIntermediate(t *testing.T) {
	skipIfCaseSensitive(t)

	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "Dir", "file")
	touch(t, expected)

	// Access with wrong case in intermediate component
	actual, err := Canonical(filepath.Join(tmp, "dIR", "file"))
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_CaseSensitiveNoFallback(t *testing.T) {
	skipIfCaseInsensitive(t)

	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "foo"))

	// Wrong case must NOT silently resolve on a case-sensitive FS.
	_, err := Canonical(filepath.Join(tmp, "Foo"))
	assert.True(t, errors.Is(err, os.ErrNotExist), "expected ErrNotExist, got: %v", err)
}

func TestCanonical_CaseSensitiveExactMatch(t *testing.T) {
	skipIfCaseInsensitive(t)

	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "Foo")
	mkdirp(t, expected)

	actual, err := Canonical(expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_CaseSensitiveIntermediate(t *testing.T) {
	skipIfCaseInsensitive(t)

	tmp := canonicalTempDir(t)
	touch(t, filepath.Join(tmp, "lower", "child"))

	// Wrong case on intermediate component must fail on case-sensitive FS.
	_, err := Canonical(filepath.Join(tmp, "Lower", "child"))
	assert.True(t, errors.Is(err, os.ErrNotExist), "expected ErrNotExist, got: %v", err)
}

func TestWeaklyCanonical_CaseSensitiveNoFallback(t *testing.T) {
	skipIfCaseInsensitive(t)

	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "foo"))

	// "Foo" must NOT match "foo" on a case-sensitive FS.
	// WeaklyCanonical treats the non-matching component as non-existent.
	actual, err := WeaklyCanonical(filepath.Join(tmp, "Foo", "missing"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "Foo", "missing"), actual)
}

// Category 8: Windows-specific =============================================

func TestCanonical_WrongSlash(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only relevant on Windows")
	}

	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "file")
	touch(t, expected)

	// Use forward slashes on Windows
	query := strings.ReplaceAll(expected, `\`, `/`)
	actual, err := Canonical(query)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCanonical_RootRelativePath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only relevant on Windows")
	}

	tmp := canonicalTempDir(t)
	// Strip the volume name to get a root-relative path: C:\foo\bar -> \foo\bar
	vol := filepath.VolumeName(tmp)
	rootRelative := tmp[len(vol):]

	actual, err := Canonical(rootRelative)
	require.NoError(t, err)
	assert.Equal(t, tmp, actual)
}

func TestWeaklyCanonical_RootRelativePath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only relevant on Windows")
	}

	tmp := canonicalTempDir(t)
	vol := filepath.VolumeName(tmp)
	rootRelative := tmp[len(vol):]

	// Existing path
	actual, err := WeaklyCanonical(rootRelative)
	require.NoError(t, err)
	assert.Equal(t, tmp, actual)

	// With non-existent tail
	query := rootRelative + string(os.PathSeparator) + "missing"
	actual, err = WeaklyCanonical(query)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "missing"), actual)
}

func TestCanonical_DriveRelativePath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only relevant on Windows")
	}

	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "file")
	touch(t, expected)

	// Drive-relative: C:path where C is the current drive.
	vol := filepath.VolumeName(tmp)
	t.Chdir(tmp)
	query := vol + "file"

	actual, err := Canonical(query)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

// Category 9: Platform edge cases ==========================================

func TestCanonical_RootPath(t *testing.T) {
	var root string
	if runtime.GOOS == "windows" {
		root = filepath.VolumeName(os.TempDir()) + `\`
	} else {
		root = "/"
	}

	actual, err := Canonical(root)
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(root), actual)
}

func TestCanonical_DevNull(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/null on Windows")
	}

	actual, err := Canonical("/dev/null")
	require.NoError(t, err)
	assert.Equal(t, "/dev/null", actual)
}

// WeaklyCanonical tests ====================================================

func TestWeaklyCanonical_FullyExists(t *testing.T) {
	tmp := canonicalTempDir(t)
	expected := filepath.Join(tmp, "dir", "file")
	touch(t, expected)

	actual, err := WeaklyCanonical(expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestWeaklyCanonical_MissingTail(t *testing.T) {
	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "dir"))

	actual, err := WeaklyCanonical(filepath.Join(tmp, "dir", "missing", "file"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "dir", "missing", "file"), actual)
}

func TestWeaklyCanonical_MissingMultipleComponents(t *testing.T) {
	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "existing"))

	actual, err := WeaklyCanonical(filepath.Join(tmp, "existing", "a", "b", "c"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "existing", "a", "b", "c"), actual)
}

func TestWeaklyCanonical_EntirelyMissing(t *testing.T) {
	// Build a platform-appropriate absolute path that doesn't exist.
	var query string
	if runtime.GOOS == "windows" {
		vol := filepath.VolumeName(os.TempDir())
		query = vol + string(os.PathSeparator) + filepath.Join("nonexistent", "path", "to", "file")
	} else {
		query = "/nonexistent/path/to/file"
	}
	actual, err := WeaklyCanonical(query)
	require.NoError(t, err)
	assert.Equal(t, query, actual)
}

func TestWeaklyCanonical_EmptyString(t *testing.T) {
	_, err := WeaklyCanonical("")
	assert.Error(t, err)
}

func TestWeaklyCanonical_Relative(t *testing.T) {
	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "dir"))

	t.Chdir(tmp)
	actual, err := WeaklyCanonical(filepath.Join("dir", "missing"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "dir", "missing"), actual)
}

func TestWeaklyCanonical_SymlinkThenMissing(t *testing.T) {
	tmp := canonicalTempDir(t)
	target := filepath.Join(tmp, "real")
	mkdirp(t, target)
	makeSymlink(t, "real", filepath.Join(tmp, "link"))

	actual, err := WeaklyCanonical(filepath.Join(tmp, "link", "missing"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "real", "missing"), actual)
}

func TestWeaklyCanonical_DotDotInTail(t *testing.T) {
	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "existing"))

	// .. in the non-existent tail is preserved because "missing" could be a
	// symlink, making lexical collapsing incorrect.
	query := filepath.Join(tmp, "existing") + string(os.PathSeparator) +
		"missing" + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "other"
	actual, err := WeaklyCanonical(query)
	require.NoError(t, err)
	expected := filepath.Join(tmp, "existing") + string(os.PathSeparator) +
		"missing" + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "other"
	assert.Equal(t, expected, actual)
}

func TestWeaklyCanonical_CaseNormalized(t *testing.T) {
	skipIfCaseSensitive(t)

	tmp := canonicalTempDir(t)
	mkdirp(t, filepath.Join(tmp, "Dir"))

	// Existing prefix gets case-normalized; tail preserves input case.
	actual, err := WeaklyCanonical(filepath.Join(tmp, "dIR", "Missing"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "Dir", "Missing"), actual)
}

func TestWeaklyCanonical_BrokenSymlink(t *testing.T) {
	tmp := canonicalTempDir(t)
	makeSymlink(t, "nonexistent", filepath.Join(tmp, "broken"))

	// Broken symlink: the symlink itself exists but its target doesn't.
	// WeaklyCanonical should not error — it returns the resolved prefix + the name.
	actual, err := WeaklyCanonical(filepath.Join(tmp, "broken"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "nonexistent"), actual)
}
