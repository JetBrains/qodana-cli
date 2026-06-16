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

// Standard dotenv expansion: a single-quoted value is literal, while an unquoted (or double-quoted)
// value has $NAME / ${NAME} expanded from the environment. Locks the contract that CONTRIBUTING and
// .env.example rely on (single-quote a token so a literal $ is not mistaken for a variable reference).
func TestRead_SingleQuoteKeepsValueLiteral(t *testing.T) {
	t.Setenv("INJECTED", "boom")
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(p, []byte("LITERAL='tok$INJECTED-end'\nEXPANDED=tok$INJECTED-end\n"), 0o644))

	m, err := Read(p)
	require.NoError(t, err)
	assert.Equal(t, "tok$INJECTED-end", m["LITERAL"], "single-quoted value must stay literal")
	assert.Equal(t, "tokboom-end", m["EXPANDED"], "unquoted value expands $NAME from the environment")
}
