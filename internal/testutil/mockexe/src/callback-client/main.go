// Package main is a callback client for the mockexe test framework.
// It connects back to the test process via TCP using a binary framing
// protocol, forwarding stdin/stdout/stderr as streams.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/JetBrains/qodana-cli/internal/testutil/mockexe"
)

const callbackTimeout = 30 * time.Second

// addr is the callback server address (host:port), set at build time
// via -ldflags "-X main.addr=...". MOCKEXE_ADDR env var takes precedence.
var addr string

func main() {
	os.Exit(run())
}

func run() int {
	if envAddr := os.Getenv("MOCKEXE_ADDR"); envAddr != "" {
		addr = envAddr
	}
	if addr == "" {
		fmt.Fprintf(os.Stderr, "mockexe: callback address not set (neither MOCKEXE_ADDR nor -ldflags)\n")
		return 1
	}

	conn, err := net.DialTimeout("tcp", addr, callbackTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mockexe: cannot connect to %s: %v\n", addr, err)
		return 1
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(callbackTimeout))

	// Send init frame.
	init := mockexe.InitPayload{Argv: os.Args, Env: os.Environ()}
	initData, err := json.Marshal(init)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mockexe: failed to marshal init: %v\n", err)
		return 1
	}
	if err := mockexe.WriteFrame(conn, mockexe.FrameInit, initData); err != nil {
		fmt.Fprintf(os.Stderr, "mockexe: failed to send init: %v\n", err)
		return 1
	}

	// Forward stdin in a background goroutine.
	var writeMu sync.Mutex
	go forwardStdin(conn, &writeMu)

	// Read frames from server until Exit.
	for {
		typ, payload, err := mockexe.ReadFrame(conn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mockexe: failed to read frame: %v\n", err)
			return 1
		}
		switch typ {
		case mockexe.FrameStdout:
			if len(payload) > 0 {
				os.Stdout.Write(payload)
			}
		case mockexe.FrameStderr:
			if len(payload) > 0 {
				os.Stderr.Write(payload)
			}
		case mockexe.FrameExit:
			return mockexe.UnmarshalExitCode(payload)
		}
	}
}

func forwardStdin(conn net.Conn, mu *sync.Mutex) {
	buf := make([]byte, 32*1024)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			mu.Lock()
			_ = mockexe.WriteFrame(conn, mockexe.FrameStdin, buf[:n])
			mu.Unlock()
		}
		if err != nil {
			mu.Lock()
			_ = mockexe.WriteFrame(conn, mockexe.FrameStdin, nil)
			mu.Unlock()
			return
		}
	}
}
