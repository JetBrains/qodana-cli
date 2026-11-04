/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	foundationexec "github.com/JetBrains/qodana-cli/internal/foundation/exec"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	// The server announces its endpoint with this line once it serves MCP.
	edictServerReadyLine = "edict-mcp listening at "
	// The JVM starts within seconds; the IDE starts later, on the first inspection call.
	edictServerStartTimeout = 2 * time.Minute
	// The server stops its IDE on termination, which can take as long as the IDE's own shutdown.
	edictServerStopTimeout = time.Minute
)

// edictServerFiles are the log and the process record of the detached server of one project. They live in the user
// cache, keyed by the project's real path, so that `edict mcp stop` finds the server without reading the project.
type edictServerFiles struct {
	log    string
	record string
}

func newEdictServerFiles(projectDir string) (edictServerFiles, error) {
	project, err := filepath.Abs(projectDir)
	if err != nil {
		return edictServerFiles{}, err
	}
	if project, err = filepath.EvalSymlinks(project); err != nil {
		return edictServerFiles{}, err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return edictServerFiles{}, fmt.Errorf("resolve the Edict server directory: %w", err)
	}
	hash := sha256.Sum256([]byte(project))
	directory := filepath.Join(cache, "JetBrains", "Qodana", "edict", "servers", hex.EncodeToString(hash[:8]))
	return edictServerFiles{
		log:    filepath.Join(directory, "server.log"),
		record: filepath.Join(directory, "server.pid"),
	}, nil
}

// edictServerRecord identifies the detached server process: a pid alone could name a later process that reuses it.
type edictServerRecord struct {
	pid       int
	createdAt int64
}

func (files edictServerFiles) write(record edictServerRecord) error {
	return os.WriteFile(files.record, []byte(fmt.Sprintf("%d %d\n", record.pid, record.createdAt)), 0o600)
}

// running returns the recorded server while it runs; a record of an exited process is removed.
func (files edictServerFiles) running() (*process.Process, error) {
	data, err := os.ReadFile(files.record)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 2 {
		pid, pidErr := strconv.Atoi(fields[0])
		createdAt, timeErr := strconv.ParseInt(fields[1], 10, 64)
		if pidErr == nil && timeErr == nil {
			if server, err := process.NewProcess(int32(pid)); err == nil {
				if actual, err := server.CreateTime(); err == nil && actual == createdAt && !exited(server) {
					return server, nil
				}
			}
		}
	}
	return nil, os.Remove(files.record)
}

// startDetachedEdictServer runs arguments, a foreground `edict mcp start`, in a session of its own with output in the
// server log, and returns once it announces its endpoint. The server keeps running after this process exits.
func startDetachedEdictServer(executable string, arguments []string, files edictServerFiles, stdout io.Writer) error {
	if server, err := files.running(); err != nil {
		return err
	} else if server != nil {
		return fmt.Errorf("an Edict server already runs for this project (pid %d); stop it with `qodana edict mcp stop`", server.Pid)
	}
	if err := os.MkdirAll(filepath.Dir(files.log), 0o700); err != nil {
		return err
	}
	log, err := os.Create(files.log)
	if err != nil {
		return err
	}
	defer log.Close()
	server := exec.Command(executable, arguments...)
	server.Stdout, server.Stderr = log, log
	server.SysProcAttr = detachedProcessAttributes()
	if err := server.Start(); err != nil {
		return fmt.Errorf("start the Edict server: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- server.Wait() }()
	url, err := awaitEdictServer(files.log, exited)
	if err != nil {
		return fmt.Errorf("%w; server log %s:\n%s", err, files.log, tail(files.log, 40))
	}
	started, err := process.NewProcess(int32(server.Process.Pid))
	if err != nil {
		return err
	}
	createdAt, err := started.CreateTime()
	if err != nil {
		return err
	}
	if err := files.write(edictServerRecord{pid: server.Process.Pid, createdAt: createdAt}); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%s%s\nServer log: %s\nStop it with `qodana edict mcp stop`.\n", edictServerReadyLine, url, files.log)
	return err
}

// awaitEdictServer returns the endpoint the server announces in its log, or fails if it exits or does not announce one.
func awaitEdictServer(log string, exited <-chan error) (string, error) {
	deadline := time.After(edictServerStartTimeout)
	poll := time.NewTicker(200 * time.Millisecond)
	defer poll.Stop()
	for {
		if url, ok := edictServerEndpoint(log); ok {
			return url, nil
		}
		select {
		case err := <-exited:
			if url, ok := edictServerEndpoint(log); ok {
				return "", fmt.Errorf("the Edict server announced %s and exited: %v", url, err)
			}
			return "", fmt.Errorf("the Edict server exited before serving MCP: %v", err)
		case <-deadline:
			return "", fmt.Errorf("the Edict server did not serve MCP within %s", edictServerStartTimeout)
		case <-poll.C:
		}
	}
}

func edictServerEndpoint(log string) (string, bool) {
	file, err := os.Open(log)
	if err != nil {
		return "", false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if _, url, ok := strings.Cut(scanner.Text(), edictServerReadyLine); ok {
			return strings.TrimSpace(url), true
		}
	}
	return "", false
}

// stopEdictServer terminates the recorded server, which stops its IDE, and kills its session after timeout.
func stopEdictServer(files edictServerFiles, timeout time.Duration, stdout io.Writer) error {
	server, err := files.running()
	if err != nil {
		return err
	}
	if server == nil {
		_, err := fmt.Fprintln(stdout, "No Edict server runs for this project")
		return err
	}
	pid := int(server.Pid)
	if osProcess, err := os.FindProcess(pid); err == nil {
		if err := foundationexec.RequestTermination(osProcess); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("stop the Edict server (pid %d): %w", pid, err)
		}
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) && !exited(server) {
		time.Sleep(200 * time.Millisecond)
	}
	if !exited(server) {
		if err := killProcessSession(pid); err != nil {
			return fmt.Errorf("kill the Edict server (pid %d) after %s: %w", pid, timeout, err)
		}
		_, _ = fmt.Fprintf(stdout, "Killed the Edict server (pid %d), which did not stop within %s\n", pid, timeout)
	} else {
		_, _ = fmt.Fprintf(stdout, "Stopped the Edict server (pid %d)\n", pid)
	}
	if err := os.Remove(files.record); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// exited reports whether the process has exited. A detached server outlives the `edict mcp start` that started it, so
// when it exits its parent is the session's init process, which may never reap it: under a container's
// `tail -f /dev/null` the server stays a zombie until the container ends.
func exited(server *process.Process) bool {
	if running, err := server.IsRunning(); err != nil || !running {
		return true
	}
	status, err := server.Status()
	return err == nil && slices.Contains(status, process.Zombie)
}

func tail(path string, lines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	all := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	return strings.Join(all[max(0, len(all)-lines):], "\n")
}
