package fs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelativity_Absolute(t *testing.T) {
	if runtime.GOOS == "windows" {
		assert.Equal(t, Absolute, Relativity(`C:\Users`))
		assert.Equal(t, Absolute, Relativity(`D:\`))
		assert.Equal(t, Absolute, Relativity(`\\server\share`))
		assert.Equal(t, Absolute, Relativity(`\\server\share\file`))
	} else {
		assert.Equal(t, Absolute, Relativity("/"))
		assert.Equal(t, Absolute, Relativity("/usr/bin"))
		assert.Equal(t, Absolute, Relativity("/tmp"))
	}
}

func TestRelativity_Relative(t *testing.T) {
	assert.Equal(t, Relative, Relativity("foo"))
	assert.Equal(t, Relative, Relativity("foo/bar"))
	assert.Equal(t, Relative, Relativity("./foo"))
	assert.Equal(t, Relative, Relativity("../foo"))
	assert.Equal(t, Relative, Relativity("."))
	assert.Equal(t, Relative, Relativity(".."))
	assert.Equal(t, Relative, Relativity(""))
}

func TestRelativity_RootRelative(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("root-relative paths only exist on Windows")
	}

	assert.Equal(t, RootRelative, Relativity(`\Users`))
	assert.Equal(t, RootRelative, Relativity(`\`))
	assert.Equal(t, RootRelative, Relativity(`/Users`))
	assert.Equal(t, RootRelative, Relativity(`/`))
}

func TestRelativity_DriveRelative(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive-relative paths only exist on Windows")
	}

	assert.Equal(t, DriveRelative, Relativity(`C:foo`))
	assert.Equal(t, DriveRelative, Relativity(`D:bar`))
	assert.Equal(t, DriveRelative, Relativity(`C:.`))
	assert.Equal(t, DriveRelative, Relativity(`C:..`))
}

func TestRelativity_String(t *testing.T) {
	assert.Equal(t, "Absolute", Absolute.String())
	assert.Equal(t, "Relative", Relative.String())
	assert.Equal(t, "RootRelative", RootRelative.String())
	assert.Equal(t, "DriveRelative", DriveRelative.String())
}

func TestRelativity_UnixBackslash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("backslash is a separator on Windows")
	}

	// On Unix, backslash is a valid filename character, not a separator.
	assert.Equal(t, Relative, Relativity(`\foo`))
	assert.Equal(t, Relative, Relativity(`C:\foo`))
	assert.Equal(t, Relative, Relativity(`C:foo`))
}

func TestMakeAbsolute_AlreadyAbsolute(t *testing.T) {
	var path string
	if runtime.GOOS == "windows" {
		path = `C:\Users\test`
	} else {
		path = "/usr/bin"
	}
	result, err := MakeAbsolute(path)
	require.NoError(t, err)
	assert.Equal(t, path, result)
}

func TestMakeAbsolute_Relative(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	result, err := MakeAbsolute("foo")
	require.NoError(t, err)
	assert.Equal(t, cwd+string(os.PathSeparator)+"foo", result)
}

func TestMakeAbsolute_RelativeDot(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	result, err := MakeAbsolute(".")
	require.NoError(t, err)
	assert.Equal(t, cwd+string(os.PathSeparator)+".", result)
}

func TestMakeAbsolute_RootRelative(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("root-relative paths only exist on Windows")
	}

	cwd, err := os.Getwd()
	require.NoError(t, err)
	vol := filepath.VolumeName(cwd)

	result, err := MakeAbsolute(`\Users`)
	require.NoError(t, err)
	assert.Equal(t, vol+`\Users`, result)
}

func TestMakeAbsolute_DriveRelative(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive-relative paths only exist on Windows")
	}

	tmp := t.TempDir()
	touch(t, filepath.Join(tmp, "file"))
	vol := filepath.VolumeName(tmp)
	t.Chdir(tmp)

	result, err := MakeAbsolute(vol + "file")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "file"), result)
}

// Join tests ===============================================================

func TestJoin_Basic(t *testing.T) {
	sep := string(os.PathSeparator)
	assert.Equal(t, "a"+sep+"b", Join("a", "b"))
	assert.Equal(t, "a"+sep+"b"+sep+"c", Join("a", "b", "c"))
}

func TestJoin_EmptyElements(t *testing.T) {
	sep := string(os.PathSeparator)
	assert.Equal(t, "a"+sep+"b", Join("a", "", "b"))
	assert.Equal(t, "a", Join("", "a", ""))
	assert.Equal(t, "", Join("", "", ""))
	assert.Equal(t, "", Join())
}

func TestJoin_PreservesAbsolute(t *testing.T) {
	sep := string(os.PathSeparator)
	if runtime.GOOS == "windows" {
		assert.Equal(t, `C:\Users\foo`, Join(`C:\Users`, "foo"))
	} else {
		assert.Equal(t, "/usr"+sep+"bin", Join("/usr", "bin"))
		assert.Equal(t, "/a", Join("/", "a"))
	}
}

func TestJoin_CollapsesDuplicateSeparators(t *testing.T) {
	sep := string(os.PathSeparator)
	assert.Equal(t, "a"+sep+"b", Join("a"+sep, sep+"b"))
}

func TestJoin_PreservesDotDot(t *testing.T) {
	sep := string(os.PathSeparator)
	// Unlike filepath.Join, we do NOT collapse .. — that's the whole point.
	assert.Equal(t, "a"+sep+"link"+sep+"..", Join("a", "link", ".."))
}

func TestJoin_NoTrailingSeparator(t *testing.T) {
	sep := string(os.PathSeparator)
	result := Join("a", "b"+sep)
	assert.Equal(t, "a"+sep+"b", result)
}

// Dir tests ================================================================

func TestDir_Basic(t *testing.T) {
	sep := string(os.PathSeparator)
	assert.Equal(t, "a", Dir("a"+sep+"b"))
	assert.Equal(t, "a"+sep+"b", Dir("a"+sep+"b"+sep+"c"))
}

func TestDir_Root(t *testing.T) {
	if runtime.GOOS == "windows" {
		assert.Equal(t, `C:\`, Dir(`C:\foo`))
		assert.Equal(t, `C:\`, Dir(`C:\`))
	} else {
		assert.Equal(t, "/", Dir("/foo"))
		assert.Equal(t, "/", Dir("/"))
	}
}

func TestDir_NoSeparator(t *testing.T) {
	assert.Equal(t, ".", Dir("foo"))
	assert.Equal(t, ".", Dir("."))
}

func TestDir_PreservesDotDot(t *testing.T) {
	sep := string(os.PathSeparator)
	// Unlike filepath.Dir, we do NOT resolve .. — that's the whole point.
	assert.Equal(t, "a"+sep+"link", Dir("a"+sep+"link"+sep+".."))
}
