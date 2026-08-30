/*
 * Copyright 2021-2024 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package mcp manages the lifecycle of the MCP server shipped with a Qodana linter.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JetBrains/qodana-cli/internal/foundation/flock"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
)

const StateVersion = 1

type Ready struct {
	Status string `json:"status"`
	URL    string `json:"url"`
}

type State struct {
	Version    int       `json:"version"`
	PID        int32     `json:"pid"`
	URL        string    `json:"url"`
	ProjectDir string    `json:"projectDir"`
	Executable string    `json:"executable"`
	StartedAt  time.Time `json:"startedAt"`
	LogFile    string    `json:"logFile"`
	Linter     string    `json:"linter,omitempty"`
	IDE        string    `json:"ide,omitempty"`
	RuntimeDir string    `json:"runtimeDir,omitempty"`
}

type StartOptions struct {
	ProjectDir  string
	StateFile   string
	ReadyFile   string
	LogFile     string
	Linter      string
	IDE         string
	Property    []string
	Port        int
	WaitTimeout time.Duration
}

type LaunchRequest struct {
	ProjectDir string
	ReadyFile  string
	LogFile    string
	Linter     string
	IDE        string
	Property   []string
	Port       int
}

type Process interface {
	PID() int32
	Executable() string
	RuntimeDir() string
	Done() <-chan error
	Terminate() error
	Kill() error
}

type Launcher interface {
	Launch(context.Context, LaunchRequest) (Process, error)
}

type ProcessController interface {
	Matches(State) (running bool, matches bool, err error)
	Terminate(pid int32) error
	Kill(pid int32) error
}

type Service struct {
	Launcher     Launcher
	Processes    ProcessController
	PollInterval time.Duration
}

type StatusResult struct {
	Status    string `json:"status"`
	StateFile string `json:"stateFile"`
	State
}

func (s Service) Start(ctx context.Context, options StartOptions) (State, error) {
	if s.Launcher == nil || s.Processes == nil {
		return State{}, errors.New("MCP lifecycle dependencies are not configured")
	}
	if options.ProjectDir == "" || options.StateFile == "" || options.ReadyFile == "" || options.LogFile == "" {
		return State{}, errors.New("project, state, readiness, and log paths are required")
	}
	if options.Port < 0 || options.Port > 65535 {
		return State{}, fmt.Errorf("invalid MCP port %d", options.Port)
	}
	if options.WaitTimeout <= 0 {
		return State{}, errors.New("MCP wait timeout must be positive")
	}

	var result State
	var operationErr error
	lockErr := flock.With(options.StateFile+".lock", func() {
		result, operationErr = s.startLocked(ctx, options)
	})
	return result, errors.Join(operationErr, lockErr)
}

func (s Service) startLocked(ctx context.Context, options StartOptions) (State, error) {
	if existing, err := LoadState(options.StateFile); err == nil {
		running, matches, inspectErr := s.Processes.Matches(existing)
		if inspectErr != nil {
			return State{}, fmt.Errorf("checking existing MCP process: %w", inspectErr)
		}
		if running && matches {
			return State{}, fmt.Errorf("MCP server is already running with PID %d", existing.PID)
		}
		if running {
			return State{}, fmt.Errorf("state file points to PID %d owned by another process", existing.PID)
		}
		if err := removeRuntimeDir(existing.RuntimeDir); err != nil {
			return State{}, err
		}
		if err := os.Remove(options.StateFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return State{}, fmt.Errorf("removing stale MCP state: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return State{}, err
	}

	for _, path := range []string{options.StateFile, options.ReadyFile, options.LogFile} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return State{}, fmt.Errorf("creating directory for %s: %w", path, err)
		}
	}
	if err := os.Remove(options.ReadyFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return State{}, fmt.Errorf("removing stale MCP readiness file: %w", err)
	}

	process, err := s.Launcher.Launch(ctx, LaunchRequest{
		ProjectDir: options.ProjectDir,
		ReadyFile:  options.ReadyFile,
		LogFile:    options.LogFile,
		Linter:     options.Linter,
		IDE:        options.IDE,
		Property:   options.Property,
		Port:       options.Port,
	})
	if err != nil {
		return State{}, fmt.Errorf("starting MCP server: %w", err)
	}

	ready, err := s.waitUntilReady(ctx, process, options.ReadyFile, options.LogFile, options.WaitTimeout)
	if err != nil {
		s.stopStartedProcess(process)
		return State{}, err
	}
	state := State{
		Version: StateVersion, PID: process.PID(), URL: ready.URL,
		ProjectDir: options.ProjectDir, Executable: process.Executable(), StartedAt: time.Now().UTC(),
		LogFile: options.LogFile, Linter: options.Linter, IDE: options.IDE, RuntimeDir: process.RuntimeDir(),
	}
	if err := WriteState(options.StateFile, state); err != nil {
		s.stopStartedProcess(process)
		return State{}, err
	}
	_ = os.Remove(options.ReadyFile)
	return state, nil
}

func (s Service) waitUntilReady(
	ctx context.Context,
	process Process,
	readyFile string,
	logFile string,
	timeout time.Duration,
) (Ready, error) {
	pollInterval := s.PollInterval
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		if ready, found, err := readReady(readyFile); err != nil {
			return Ready{}, fmt.Errorf("reading MCP readiness: %w", err)
		} else if found {
			select {
			case err := <-process.Done():
				if err == nil {
					err = errors.New("process exited")
				}
				return Ready{}, fmt.Errorf("MCP server exited while becoming ready: %w", err)
			default:
				return ready, nil
			}
		}
		select {
		case <-ctx.Done():
			return Ready{}, fmt.Errorf("waiting for MCP readiness: %w", ctx.Err())
		case err := <-process.Done():
			if err == nil {
				err = errors.New("process exited")
			}
			return Ready{}, fmt.Errorf("MCP server exited before becoming ready: %w", err)
		case <-timer.C:
			return Ready{}, fmt.Errorf("MCP server did not become ready within %s; see %s", timeout, logFile)
		case <-ticker.C:
		}
	}
}

func (s Service) stopStartedProcess(process Process) {
	_ = process.Terminate()
	select {
	case <-process.Done():
	case <-time.After(2 * time.Second):
		_ = process.Kill()
	}
}

func (s Service) Status(stateFile string) (StatusResult, error) {
	state, err := LoadState(stateFile)
	if errors.Is(err, os.ErrNotExist) {
		return StatusResult{Status: "stopped", StateFile: stateFile}, nil
	}
	if err != nil {
		return StatusResult{}, err
	}
	running, matches, err := s.Processes.Matches(state)
	if err != nil {
		return StatusResult{}, fmt.Errorf("checking MCP process: %w", err)
	}
	status := "stale"
	if running && matches {
		status = "running"
	} else if running {
		status = "pid-reused"
	}
	return StatusResult{Status: status, StateFile: stateFile, State: state}, nil
}

func (s Service) Stop(ctx context.Context, stateFile string, timeout time.Duration) (StatusResult, error) {
	var result StatusResult
	var operationErr error
	lockErr := flock.With(stateFile+".lock", func() {
		result, operationErr = s.stopLocked(ctx, stateFile, timeout)
	})
	return result, errors.Join(operationErr, lockErr)
}

func (s Service) stopLocked(ctx context.Context, stateFile string, timeout time.Duration) (StatusResult, error) {
	status, err := s.Status(stateFile)
	if err != nil {
		return StatusResult{}, err
	}
	if status.Status == "stopped" || status.Status == "stale" {
		if err := removeRuntimeDir(status.RuntimeDir); err != nil {
			return status, err
		}
		_ = os.Remove(stateFile)
		status.Status = "stopped"
		return status, nil
	}
	if status.Status == "pid-reused" {
		return status, fmt.Errorf("refusing to stop PID %d because its executable does not match the state file", status.PID)
	}
	if err := s.Processes.Terminate(status.PID); err != nil {
		return status, fmt.Errorf("requesting MCP process termination: %w", err)
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(s.pollInterval())
	defer ticker.Stop()
	for {
		running, matches, inspectErr := s.Processes.Matches(status.State)
		if inspectErr != nil {
			return status, inspectErr
		}
		if !running || !matches {
			break
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-deadline.C:
			if err := s.Processes.Kill(status.PID); err != nil {
				return status, fmt.Errorf("killing MCP process: %w", err)
			}
			goto stopped
		case <-ticker.C:
		}
	}

stopped:
	if err := removeRuntimeDir(status.RuntimeDir); err != nil {
		return status, err
	}
	if err := os.Remove(stateFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return status, fmt.Errorf("removing MCP state: %w", err)
	}
	status.Status = "stopped"
	return status, nil
}

func removeRuntimeDir(path string) error {
	if path == "" {
		return nil
	}
	cleanPath := filepath.Clean(path)
	if filepath.Dir(cleanPath) != filepath.Clean(os.TempDir()) ||
		!strings.HasPrefix(filepath.Base(cleanPath), "qodana-mcp-config-") {
		return fmt.Errorf("refusing to remove unexpected MCP runtime directory %s", path)
	}
	if err := os.RemoveAll(cleanPath); err != nil {
		return fmt.Errorf("removing MCP runtime directory: %w", err)
	}
	return nil
}

func (s Service) pollInterval() time.Duration {
	if s.PollInterval > 0 {
		return s.PollInterval
	}
	return 100 * time.Millisecond
}

func LoadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("parsing MCP state file %s: %w", path, err)
	}
	if state.Version != StateVersion || state.PID <= 0 || state.Executable == "" {
		return State{}, fmt.Errorf("invalid MCP state file %s", path)
	}
	return state, nil
}

func WriteState(path string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := fs.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("writing MCP state file: %w", err)
	}
	return nil
}

func WriteReady(path string, ready Ready) error {
	data, err := json.Marshal(ready)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := fs.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("writing MCP readiness file: %w", err)
	}
	return nil
}

func readReady(path string) (Ready, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Ready{}, false, nil
	}
	if err != nil {
		return Ready{}, false, err
	}
	var ready Ready
	if err := json.Unmarshal(data, &ready); err != nil {
		return Ready{}, false, err
	}
	parsed, err := url.ParseRequestURI(ready.URL)
	if err != nil || ready.Status != "ready" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return Ready{}, false, errors.New("readiness file does not contain a valid ready HTTP endpoint")
	}
	return ready, true, nil
}
