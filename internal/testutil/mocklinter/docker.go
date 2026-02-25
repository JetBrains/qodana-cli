package mocklinter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/qdcontainer"
	"github.com/JetBrains/qodana-cli/internal/testutil/mockexe"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
)

// ContainerExitError is returned by RunContainer when the container exits
// with a non-zero status code.
type ContainerExitError struct {
	Code   int
	Output []byte
}

func (e *ContainerExitError) Error() string {
	return fmt.Sprintf("container exited with code %d", e.Code)
}

func (e *ContainerExitError) ExitCode() int { return e.Code }

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
	mockexe.BuildCallbackClient(t, binaryPath, "127.0.0.1:"+port,
		"linux/"+runtime.GOARCH,
	)

	return binaryPath
}

// RunContainer runs the callback-client binary inside an Alpine container
// Returns combined stdout+stderr output and any error.
func RunContainer(t testing.TB, callbackBinary string, cmdArgs ...string) ([]byte, error) {
	t.Helper()

	ctx := context.Background()
	cli, err := qdcontainer.NewContainerClient(ctx)
	if err != nil {
		t.Fatalf("mocklinter: creating Docker client: %v", err)
	}

	const imageName = "alpine:3"
	pull, err := cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return nil, fmt.Errorf("mocklinter: pulling image: %w", err)
	}
	// Drain the pull progress stream — the pull isn't complete until EOF.
	if _, err := io.Copy(io.Discard, pull); err != nil {
		return nil, fmt.Errorf("mocklinter: reading pull progress: %w", err)
	}
	if err := pull.Close(); err != nil {
		t.Logf("mocklinter: closing pull reader: %v", err)
	}

	cmd := append([]string{"/callback-client"}, cmdArgs...)

	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image: imageName,
			Cmd:   cmd,
		},
		&container.HostConfig{
			NetworkMode: network.NetworkHost,
			Mounts: []mount.Mount{
				{
					Type:     mount.TypeBind,
					Source:   callbackBinary,
					Target:   "/callback-client",
					ReadOnly: true,
				},
			},
		},
		nil, nil, "",
	)
	if err != nil {
		return nil, fmt.Errorf("mocklinter: creating container: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("mocklinter: starting container: %w", err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	var exitCode int64
	select {
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("mocklinter: waiting for container: %w", err)
		}
	case status := <-statusCh:
		exitCode = status.StatusCode
	}

	logs, err := cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("mocklinter: reading container logs: %w", err)
	}
	defer func() {
		if err := logs.Close(); err != nil {
			t.Logf("mocklinter: closing logs reader: %v", err)
		}
	}()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, logs); err != nil {
		return nil, fmt.Errorf("mocklinter: copying container logs: %w", err)
	}
	output := buf.Bytes()

	// Remove container explicitly since we can't use AutoRemove (it would
	// race with ContainerLogs).
	if err := cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{}); err != nil {
		t.Logf("mocklinter: removing container: %v", err)
	}

	if exitCode != 0 {
		return output, &ContainerExitError{Code: int(exitCode), Output: output}
	}
	return output, nil
}
