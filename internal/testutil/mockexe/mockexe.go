// Package mockexe provides pre-compiled mock executables for tests.
//
// When a mock executable is run by production code, it connects back to the
// test process via TCP and triggers a registered handler closure. This lets
// tests define arbitrary mock behavior as regular Go code with full access
// to captured local variables. Stdin, stdout, and stderr are streamed in
// real time over the connection.
//
// See also: https://www.youtube.com/watch?v=0fPRO2SApO8
package mockexe

import (
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// CallbackClientPkg is the Go import path of the callback-client binary.
// Used by CreateMockExe (native) and mocklinter.Docker (cross-compiled for Linux).
const CallbackClientPkg = "github.com/JetBrains/qodana-cli/internal/testutil/mockexe/src/callback-client"

// CallContext holds the context of a mock executable invocation.
// The handler receives this when the mock binary is executed.
type CallContext struct {
	// T is a goroutine-safe [testing.TB] proxy ([MockExeT]). Use this for
	// all assertions and logging inside the handler — never use the outer t
	// captured by the closure, because its FailNow would kill the server
	// goroutine instead of stopping the handler.
	T      testing.TB
	Argv   []string          // full os.Args of the subprocess (including argv[0])
	Env    map[string]string // environment variables of the subprocess
	Stdin  io.Reader         // live stream from subprocess stdin
	Stdout io.Writer         // live stream to subprocess stdout
	Stderr io.Writer         // live stream to subprocess stderr
}

// StartCallbackServer starts a TCP server that calls handler when a callback
// client connects. It listens on listenAddr (e.g. "127.0.0.1:0").
// Returns the server address (host:port). All goroutines are cleaned up during t.Cleanup.
func StartCallbackServer(t testing.TB, listenAddr string, handler func(inv *CallContext) int) string {
	t.Helper()

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		t.Fatalf("mockexe: failed to listen on %s: %v", listenAddr, err)
	}

	var wg sync.WaitGroup
	go acceptLoop(ln, t, handler, &wg)

	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Logf("mockexe: closing listener: %v", err)
		}
		wg.Wait()
	})

	return ln.Addr().String()
}

// BuildCallbackClient compiles the callback-client binary at destPath with
// addr baked in via -ldflags. An optional platform in "os/arch" format
// (e.g. "linux/arm64") enables cross-compilation with CGO disabled.
// An empty platform targets the host.
func BuildCallbackClient(t testing.TB, destPath string, addr string, platform string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatalf("mockexe: creating directory for %s: %v", destPath, err)
	}

	cmd := exec.Command("go", "build",
		"-trimpath", // Strips build-host filesystem paths from the binary
		"-ldflags", "-s -w -X main.addr="+addr,
		"-o", destPath,
		CallbackClientPkg,
	)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if platform != "" {
		goos, goarch, ok := strings.Cut(platform, "/")
		if !ok {
			t.Fatalf("mockexe: invalid platform %q (want os/arch)", platform)
		}
		cmd.Env = append(cmd.Env, "GOOS="+goos, "GOARCH="+goarch)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mockexe: building callback client: %v\n%s", err, out)
	}
}

// CreateMockExe builds a callback-client binary at destPath with the
// callback server address baked in via -ldflags, and registers a handler
// to be called when that binary is executed.
//
// The executable can be invoked multiple times; each invocation triggers the
// handler. All goroutines are terminated when the test's cleanup runs.
//
// Warning: use ctx.T instead of captured testing.T inside the handler.
func CreateMockExe(t testing.TB, destPath string, handler func(ctx *CallContext) int) string {
	t.Helper()

	addr := StartCallbackServer(t, "127.0.0.1:0", handler)
	BuildCallbackClient(t, destPath, addr, "")

	return destPath
}
