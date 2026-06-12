package dotenv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead_ParsesWithoutMutatingEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(p, []byte("FOO=bar\n# comment\nexport BAZ=qux\n"), 0o644))

	m, err := Read(p)
	require.NoError(t, err)
	assert.Equal(t, "bar", m["FOO"])
	assert.Equal(t, "qux", m["BAZ"], "an export-prefixed line is still parsed")
	_, set := os.LookupEnv("FOO")
	assert.False(t, set, "Read must not mutate the process environment")
}

func TestRead_MissingFileIsEmptyNotError(t *testing.T) {
	m, err := Read(filepath.Join(t.TempDir(), "nope.env"))
	require.NoError(t, err)
	assert.Empty(t, m)
}

func TestValue_EnvWinsOverFileEvenWhenEmpty(t *testing.T) {
	t.Setenv("X_DOTENV_TOKEN", "")
	assert.Equal(t, "", Value("X_DOTENV_TOKEN", map[string]string{"X_DOTENV_TOKEN": "fromfile"}),
		"an explicitly-set (even empty) env var wins over the file")
	assert.Equal(t, "fromfile", Value("Y_DOTENV_TOKEN", map[string]string{"Y_DOTENV_TOKEN": "fromfile"}))
}
