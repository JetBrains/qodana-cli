package mockexe_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/testutil/mockexe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateMockExe_ExitZero(t *testing.T) {
	binPath := mockexe.CreateMockExe(t, filepath.Join(t.TempDir(), "myexe"), func(ctx *mockexe.CallContext) int {
		return 0
	})
	cmd := exec.Command(binPath)
	assert.NoError(t, cmd.Run())
}

func TestCreateMockExe_ExitNonZero(t *testing.T) {
	binPath := mockexe.CreateMockExe(t, filepath.Join(t.TempDir(), "myexe"), func(ctx *mockexe.CallContext) int {
		return 42
	})
	cmd := exec.Command(binPath)
	err := cmd.Run()
	require.Error(t, err)
	assert.Equal(t, 42, cmd.ProcessState.ExitCode())
}

func TestCreateMockExe_ForwardsArgv(t *testing.T) {
	var receivedArgv []string
	binPath := mockexe.CreateMockExe(t, filepath.Join(t.TempDir(), "myexe"), func(ctx *mockexe.CallContext) int {
		receivedArgv = ctx.Argv
		return 0
	})
	cmd := exec.Command(binPath, "hello", "world", "--flag", "value")
	assert.NoError(t, cmd.Run())
	require.Len(t, receivedArgv, 5)
	assert.Equal(t, []string{"hello", "world", "--flag", "value"}, receivedArgv[1:])
}

func TestCreateMockExe_PanicReturnsExitOne(t *testing.T) {
	binPath := mockexe.CreateMockExe(t, filepath.Join(t.TempDir(), "myexe"), func(ctx *mockexe.CallContext) int {
		panic("boom")
	})
	cmd := exec.Command(binPath)
	err := cmd.Run()
	require.Error(t, err)
	assert.Equal(t, 1, cmd.ProcessState.ExitCode())
}

func TestCreateMockExe_MultipleInvocations(t *testing.T) {
	var callCount atomic.Int32
	binPath := mockexe.CreateMockExe(t, filepath.Join(t.TempDir(), "myexe"), func(ctx *mockexe.CallContext) int {
		callCount.Add(1)
		return 0
	})
	for i := range 5 {
		cmd := exec.Command(binPath, fmt.Sprintf("call-%d", i))
		assert.NoError(t, cmd.Run(), "invocation %d should succeed", i)
		assert.Equal(t, int32(i+1), callCount.Load(), "call count after invocation %d", i)
	}
}

func TestCreateMockExe_Stdout(t *testing.T) {
	binPath := mockexe.CreateMockExe(t, filepath.Join(t.TempDir(), "myexe"), func(ctx *mockexe.CallContext) int {
		if _, err := fmt.Fprint(ctx.Stdout, "hello from mock"); err != nil {
			ctx.T.Fatalf("writing to stdout: %v", err)
		}
		return 0
	})
	out, err := exec.Command(binPath).Output()
	require.NoError(t, err)
	assert.Equal(t, "hello from mock", string(out))
}

func TestCreateMockExe_Stderr(t *testing.T) {
	binPath := mockexe.CreateMockExe(t, filepath.Join(t.TempDir(), "myexe"), func(ctx *mockexe.CallContext) int {
		if _, err := fmt.Fprint(ctx.Stderr, "error output"); err != nil {
			ctx.T.Fatalf("writing to stderr: %v", err)
		}
		return 0
	})
	cmd := exec.Command(binPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run())
	assert.Equal(t, "error output", stderr.String())
}

func TestCreateMockExe_Env(t *testing.T) {
	var receivedEnv map[string]string
	binPath := mockexe.CreateMockExe(t, filepath.Join(t.TempDir(), "myexe"), func(ctx *mockexe.CallContext) int {
		receivedEnv = ctx.Env
		return 0
	})
	cmd := exec.Command(binPath)
	cmd.Env = append(os.Environ(), "MOCK_TEST_VAR=hello123")
	require.NoError(t, cmd.Run())
	assert.Equal(t, "hello123", receivedEnv["MOCK_TEST_VAR"])
}

func TestCreateMockExe_Stdin(t *testing.T) {
	var receivedInput string
	binPath := mockexe.CreateMockExe(t, filepath.Join(t.TempDir(), "myexe"), func(ctx *mockexe.CallContext) int {
		data, _ := io.ReadAll(ctx.Stdin)
		receivedInput = string(data)
		return 0
	})
	cmd := exec.Command(binPath)
	cmd.Stdin = strings.NewReader("input data from test")
	require.NoError(t, cmd.Run())
	assert.Equal(t, "input data from test", receivedInput)
}

