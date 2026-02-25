package mocklinter

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/testutil/mockexe"
)

// Docker creates a mock Docker analyzer and returns the absolute path to the
// cross-compiled callback-client binary.
func Docker(t testing.TB, handler func(ctx *mockexe.CallContext) int) string {
	t.Helper()

	addr := mockexe.StartCallbackServer(t, "127.0.0.1:0", handler)
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("mocklinter: parsing callback address %q: %v", addr, err)
	}

	// Use .tmp/ relative to cwd — under /Users on macOS, so Docker Desktop
	// can bind-mount it (unlike /var/folders from t.TempDir()).
	if err := os.MkdirAll(".tmp", 0o755); err != nil {
		t.Fatalf("mocklinter: creating .tmp dir: %v", err)
	}
	tmpDir, err := os.MkdirTemp(".tmp", "mocklinter-")
	if err != nil {
		t.Fatalf("mocklinter: creating temp dir: %v", err)
	}
	tmpDir, err = filepath.Abs(tmpDir)
	if err != nil {
		t.Fatalf("mocklinter: resolving temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("mocklinter: removing temp dir %s: %v", tmpDir, err)
		}
	})

	binaryPath := filepath.Join(tmpDir, "callback-client-linux")
	mockexe.BuildCallbackClient(t, binaryPath, "host.docker.internal:"+port,
		"linux/"+runtime.GOARCH,
	)

	return binaryPath
}

// RunContainer runs the mock Docker container via docker run.
// Returns combined stdout+stderr output and any error.
func RunContainer(t testing.TB, callbackBinary string, cmdArgs ...string) ([]byte, error) {
	t.Helper()

	args := []string{
		"run", "--rm",
		"--add-host=host.docker.internal:host-gateway",
		"--mount", "type=bind,source=" + callbackBinary + ",target=/callback-client,readonly",
		"alpine:3", "/callback-client",
	}
	args = append(args, cmdArgs...)

	return exec.Command("docker", args...).CombinedOutput()
}
