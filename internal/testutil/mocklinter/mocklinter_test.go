package mocklinter_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/testutil/mockexe"
	"github.com/JetBrains/qodana-cli/internal/testutil/mocklinter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNative_ExitZero(t *testing.T) {
	exePath := mocklinter.Native(t, func(ctx *mockexe.CallContext) int {
		return 0
	})

	cmd := exec.Command(exePath)
	require.NoError(t, cmd.Run())
}

func TestNative_HandlerReceivesArgv(t *testing.T) {
	var receivedArgv []string
	exePath := mocklinter.Native(t, func(ctx *mockexe.CallContext) int {
		receivedArgv = ctx.Argv
		return 0
	})

	cmd := exec.Command(exePath, "qodana", "--project-dir", "/tmp")
	require.NoError(t, cmd.Run())
	require.Len(t, receivedArgv, 4)
	assert.Equal(t, []string{"qodana", "--project-dir", "/tmp"}, receivedArgv[1:])
}

func TestNative_ExitCodeForwarded(t *testing.T) {
	exePath := mocklinter.Native(t, func(ctx *mockexe.CallContext) int {
		return 42
	})

	cmd := exec.Command(exePath)
	err := cmd.Run()
	require.Error(t, err)
	assert.Equal(t, 42, cmd.ProcessState.ExitCode())
}

func skipIfNoDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
}

func TestDocker_ContainerRuns(t *testing.T) {
	skipIfNoDocker(t)

	var receivedArgv []string
	binaryPath := mocklinter.Docker(t, func(ctx *mockexe.CallContext) int {
		receivedArgv = ctx.Argv
		return 0
	})

	out, err := mocklinter.RunContainer(t, binaryPath, "hello", "world")
	require.NoError(t, err, "docker run failed: %s", out)
	require.Len(t, receivedArgv, 3)
	assert.Equal(t, []string{"hello", "world"}, receivedArgv[1:])
}

func TestDocker_ExitCodeForwarded(t *testing.T) {
	skipIfNoDocker(t)

	binaryPath := mocklinter.Docker(t, func(ctx *mockexe.CallContext) int {
		return 7
	})

	_, err := mocklinter.RunContainer(t, binaryPath)
	require.Error(t, err)
	var exitErr *mocklinter.ContainerExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.ExitCode())
}

func TestDocker_Stdout(t *testing.T) {
	skipIfNoDocker(t)

	binaryPath := mocklinter.Docker(t, func(ctx *mockexe.CallContext) int {
		_, _ = ctx.Stdout.Write([]byte("hello from container"))
		return 0
	})

	out, err := mocklinter.RunContainer(t, binaryPath)
	require.NoError(t, err)
	assert.Contains(t, string(out), "hello from container")
}

func TestDocker_WritesToHostFS(t *testing.T) {
	skipIfNoDocker(t)

	resultsDir := filepath.Join(t.TempDir(), "results")

	binaryPath := mocklinter.Docker(t, func(ctx *mockexe.CallContext) int {
		require.NoError(ctx.T, os.MkdirAll(resultsDir, 0o755))
		require.NoError(ctx.T, os.WriteFile(filepath.Join(resultsDir, "result.sarif.json"), []byte("{}"), 0o644))
		return 0
	})

	out, err := mocklinter.RunContainer(t, binaryPath)
	require.NoError(t, err, "docker run failed: %s", out)

	data, err := os.ReadFile(filepath.Join(resultsDir, "result.sarif.json"))
	require.NoError(t, err)
	assert.Equal(t, "{}", string(data))
}
