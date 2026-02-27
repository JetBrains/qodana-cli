package utils

import (
	"os"
	"strings"
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

func TestRunCmd(t *testing.T) {
	t.Run("successful command", func(t *testing.T) {
		exitCode, err := RunCmd(".", "echo", "test")
		assert.NoError(t, err)
		assert.Equal(t, 0, exitCode)
	})

	t.Run("command not found", func(t *testing.T) {
		exitCode, _ := RunCmd(".", "nonexistent_command_xyz")
		assert.NotEqual(t, 0, exitCode)
	})
}

func TestRunCmdWithTimeout(t *testing.T) {
	t.Run("command finishes before timeout", func(t *testing.T) {
		exitCode, err := RunCmdWithTimeout(".", os.Stdout, os.Stderr, 5*time.Second, 99, "echo", "test")
		assert.NoError(t, err)
		assert.Equal(t, 0, exitCode)
	})

	t.Run("timeout", func(t *testing.T) {
		exitCode, _ := RunCmdWithTimeout(".", os.Stdout, os.Stderr, 100*time.Millisecond, 99, "sleep", "5")
		assert.Equal(t, 99, exitCode)
	})
}

func TestRunCmdRedirectOutput(t *testing.T) {
	t.Run("capture stdout", func(t *testing.T) {
		stdout, stderr, exitCode, err := RunCmdRedirectOutput(".", "echo test")
		assert.NoError(t, err)
		assert.Equal(t, 0, exitCode)
		assert.Contains(t, stdout, "test")
		assert.Empty(t, stderr)
	})

	t.Run("capture stderr", func(t *testing.T) {
		stdout, stderr, exitCode, err := RunCmdRedirectOutput(".", "echo test >&2")
		assert.NoError(t, err)
		assert.Equal(t, 0, exitCode)
		assert.Empty(t, stdout)
		assert.Contains(t, stderr, "test")
	})
}

func TestGetCwdPath(t *testing.T) {
	t.Run("empty cwd uses current dir", func(t *testing.T) {
		path, err := getCwdPath("")
		assert.NoError(t, err)
		assert.NotEmpty(t, path)
	})

	t.Run("explicit cwd", func(t *testing.T) {
		dir := t.TempDir()
		path, err := getCwdPath(dir)
		assert.NoError(t, err)
		assert.Equal(t, dir, path)
	})
}

func TestClosePipe(t *testing.T) {
	t.Run("nil pipe", func(t *testing.T) {
		closePipe(nil)
	})
}

func TestClosePipes(t *testing.T) {
	t.Run("nil pipes", func(t *testing.T) {
		closePipes(nil, nil)
	})
}

func TestCopyToChannel(t *testing.T) {
	ch := make(chan string, 10)
	reader, writer, _ := os.Pipe()

	go func() {
		_, _ = writer.WriteString("test line\n")
		_ = writer.Close()
	}()

	copyToChannel(reader, ch)
	// copyToChannel closes the channel, so we just read from it

	var result strings.Builder
	for line := range ch {
		result.WriteString(line)
	}
	assert.Contains(t, result.String(), "test line")
}
