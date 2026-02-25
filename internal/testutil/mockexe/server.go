package mockexe

import (
	"encoding/json"
	"io"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"

	log "github.com/sirupsen/logrus"
)

func acceptLoop(ln net.Listener, t testing.TB, handler func(ctx *CallContext) int, wg *sync.WaitGroup) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			serveCallback(conn, t, handler)
		}()
	}
}

func serveCallback(conn net.Conn, t testing.TB, handler func(ctx *CallContext) int) {
	defer conn.Close()

	// Read init frame.
	typ, payload, err := ReadFrame(conn)
	if err != nil {
		log.Errorf("mockexe: reading init frame: %v", err)
		return
	}
	if typ != FrameInit {
		log.Errorf("mockexe: expected init frame (0x%02x), got 0x%02x", FrameInit, typ)
		return
	}
	var init InitPayload
	if err := json.Unmarshal(payload, &init); err != nil {
		log.Errorf("mockexe: unmarshalling init payload: %v", err)
		return
	}

	// Set up stdin pipe: stdinReceiver writes to pw, handler reads from pr.
	pr, pw := io.Pipe()

	// Frame writer with mutex for concurrent stdout/stderr writes.
	fw := &frameWriter{w: conn}

	// Receive stdin frames in background.
	var stdinWg sync.WaitGroup
	stdinWg.Add(1)
	go func() {
		defer stdinWg.Done()
		receiveStdin(conn, pw)
	}()

	ctx := &CallContext{
		T:      &MockExeT{t},
		Argv:   init.Argv,
		Env:    parseEnv(init.Env),
		Stdin:  pr,
		Stdout: &streamWriter{fw: fw, typ: FrameStdout},
		Stderr: &streamWriter{fw: fw, typ: FrameStderr},
	}

	exitCode := callHandlerSafe(ctx, handler)

	// Close stdin reader so receiveStdin can finish if handler didn't drain it.
	pr.Close()
	stdinWg.Wait()

	// Send stdout/stderr EOF and exit frame.
	if err := fw.writeFrame(FrameStdout, nil); err != nil {
		log.Errorf("mockexe: writing stdout EOF frame: %v", err)
	}
	if err := fw.writeFrame(FrameStderr, nil); err != nil {
		log.Errorf("mockexe: writing stderr EOF frame: %v", err)
	}

	if err := fw.writeFrame(FrameExit, MarshalExitCode(exitCode)); err != nil {
		log.Errorf("mockexe: writing exit frame: %v", err)
	}
}

func receiveStdin(conn net.Conn, pw *io.PipeWriter) {
	for {
		typ, payload, err := ReadFrame(conn)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if typ != FrameStdin {
			continue
		}
		if len(payload) == 0 {
			pw.Close()
			return
		}
		if _, err := pw.Write(payload); err != nil {
			return
		}
	}
}

func callHandlerSafe(ctx *CallContext, handler func(ctx *CallContext) int) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			switch r.(type) {
			case failNowSentinel:
				exitCode = 1
			case skipNowSentinel:
				exitCode = 0
			default:
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				ctx.T.Logf("mockexe handler panicked: %v\n%s", r, buf[:n])
				exitCode = 1
			}
		}
	}()
	return handler(ctx)
}

// parseEnv converts ["KEY=VALUE", ...] to map[string]string.
func parseEnv(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok {
			m[k] = v
		}
	}
	return m
}
