package mocklinter_test

import (
	"os/exec"
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
