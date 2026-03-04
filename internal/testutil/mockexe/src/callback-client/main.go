// Package main is a callback client for the mockexe test framework.
// It connects back to the test process via TCP using a binary framing
// protocol, forwarding stdin/stdout/stderr as streams.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/JetBrains/qodana-cli/internal/testutil/mockexe"
)

const (
	callbackTimeout = 30 * time.Second
	stdinBufSize    = 32 << 10 // 32 KB
)

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
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "mockexe: closing connection: %v\n", err)
		}
	}()

	if err := conn.SetDeadline(time.Now().Add(callbackTimeout)); err != nil {
		fmt.Fprintf(os.Stderr, "mockexe: setting deadline: %v\n", err)
		return 1
	}

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

	// Forward stdin in a background goroutine. This is the sole writer to conn;
	// the main goroutine only reads frames.
	go forwardStdin(conn)

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
				if _, err := os.Stdout.Write(payload); err != nil {
					fmt.Fprintf(os.Stderr, "mockexe: writing stdout: %v\n", err)
				}
			}
		case mockexe.FrameStderr:
			if len(payload) > 0 {
				if _, err := os.Stderr.Write(payload); err != nil {
					fmt.Fprintf(os.Stderr, "mockexe: writing stderr: %v\n", err)
				}
			}
		case mockexe.FrameExit:
			code, err := mockexe.UnmarshalExitCode(payload)
			if err != nil {
				fmt.Fprintf(os.Stderr, "mockexe: %v\n", err)
				return 1
			}
			return code
		}
	}
}

func forwardStdin(conn net.Conn) {
	buf := make([]byte, stdinBufSize)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if wErr := mockexe.WriteFrame(conn, mockexe.FrameStdin, buf[:n]); wErr != nil {
				fmt.Fprintf(os.Stderr, "mockexe: forwarding stdin: %v\n", wErr)
				return
			}
		}
		if err != nil {
			if wErr := mockexe.WriteFrame(conn, mockexe.FrameStdin, nil); wErr != nil {
				fmt.Fprintf(os.Stderr, "mockexe: sending stdin EOF: %v\n", wErr)
			}
			return
		}
	}
}
