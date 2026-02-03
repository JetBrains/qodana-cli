package utils

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBootstrap(t *testing.T) {
	t.Run("empty command", func(t *testing.T) {
		Bootstrap("", ".")
	})

	t.Run("echo command", func(t *testing.T) {
		Bootstrap("echo test", t.TempDir())
	})
}

func TestExec(t *testing.T) {
	t.Run("successful command", func(t *testing.T) {
		exitCode, err := Exec(".", "echo", "test")
		assert.NoError(t, err)
		assert.Equal(t, 0, exitCode)
	})

	t.Run("command not found", func(t *testing.T) {
		exitCode, _ := Exec(".", "nonexistent_command_xyz")
		assert.NotEqual(t, 0, exitCode)
	})
}

func TestExecWithTimeout(t *testing.T) {
	t.Run("command finishes before timeout", func(t *testing.T) {
		exitCode, err := ExecWithTimeout(".", os.Stdout, os.Stderr, 5*time.Second, 99, "echo", "test")
		assert.NoError(t, err)
		assert.Equal(t, 0, exitCode)
	})

	t.Run("timeout", func(t *testing.T) {
		exitCode, _ := ExecWithTimeout(".", os.Stdout, os.Stderr, 100*time.Millisecond, 99, "sleep", "5")
		assert.Equal(t, 99, exitCode)
	})
}

func TestExecRedirectOutput(t *testing.T) {
	t.Run("capture stdout", func(t *testing.T) {
		stdout, stderr, exitCode, err := ExecRedirectOutput(".", "echo", "test")
		assert.NoError(t, err)
		assert.Equal(t, 0, exitCode)
		assert.Contains(t, stdout, "test")
		assert.Empty(t, stderr)
	})

	t.Run("capture stderr", func(t *testing.T) {
		// Use RunShellRedirectOutput to redirect echo to stderr
		stdout, stderr, exitCode, err := RunShellRedirectOutput(".", "echo test >&2")
		assert.NoError(t, err)
		assert.Equal(t, 0, exitCode)
		assert.Empty(t, stdout)
		assert.Contains(t, stderr, "test")
	})
}


