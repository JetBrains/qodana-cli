/*
 * Copyright 2021-2024 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package mcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeProcess struct {
	pid        int32
	executable string
	done       chan error
	onStop     func()
	stopOnce   sync.Once
	terminated bool
	killed     bool
}

func (p *fakeProcess) PID() int32         { return p.pid }
func (p *fakeProcess) Executable() string { return p.executable }
func (p *fakeProcess) Done() <-chan error { return p.done }
func (p *fakeProcess) Terminate() error {
	p.terminated = true
	p.stopOnce.Do(func() {
		if p.onStop != nil {
			p.onStop()
		}
		p.done <- nil
	})
	return nil
}
func (p *fakeProcess) Kill() error {
	p.killed = true
	p.stopOnce.Do(func() { p.done <- nil })
	return nil
}

type fakeLauncher struct {
	process *fakeProcess
	launch  func(LaunchRequest)
	err     error
}

func (l fakeLauncher) Launch(_ context.Context, request LaunchRequest) (Process, error) {
	if l.launch != nil {
		l.launch(request)
	}
	return l.process, l.err
}

type fakeController struct {
	running    bool
	matches    bool
	terminated bool
	killed     bool
}

func (c *fakeController) Matches(State) (bool, bool, error) { return c.running, c.matches, nil }
func (c *fakeController) Terminate(int32) error {
	c.terminated = true
	c.running = false
	return nil
}
func (c *fakeController) Kill(int32) error {
	c.killed = true
	c.running = false
	return nil
}

func TestServiceStartWaitsForReadinessAndWritesState(t *testing.T) {
	dir := t.TempDir()
	process := &fakeProcess{pid: 42, executable: "/qodana/bin/idea", done: make(chan error, 1)}
	controller := &fakeController{}
	service := Service{
		Launcher: fakeLauncher{process: process, launch: func(request LaunchRequest) {
			go func() {
				time.Sleep(20 * time.Millisecond)
				if err := os.WriteFile(request.ReadyFile, []byte(`{"status":"ready","url":"http://127.0.0.1:64342/mcp"}`), 0o600); err != nil {
					t.Error(err)
				}
			}()
		}},
		Processes: controller, PollInterval: time.Millisecond,
	}
	options := testStartOptions(dir)

	state, err := service.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if state.PID != 42 || state.URL != "http://127.0.0.1:64342/mcp" {
		t.Fatalf("unexpected state: %+v", state)
	}
	stored, err := LoadState(options.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PID != state.PID || stored.ProjectDir != options.ProjectDir {
		t.Fatalf("unexpected stored state: %+v", stored)
	}
	if _, err := os.Stat(options.ReadyFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("readiness file was not removed: %v", err)
	}
}

func TestServiceStartCleansUpOnTimeout(t *testing.T) {
	dir := t.TempDir()
	process := &fakeProcess{pid: 42, executable: "/qodana/bin/idea", done: make(chan error, 1)}
	service := Service{
		Launcher: fakeLauncher{process: process}, Processes: &fakeController{}, PollInterval: time.Millisecond,
	}
	options := testStartOptions(dir)
	options.WaitTimeout = 10 * time.Millisecond

	_, err := service.Start(context.Background(), options)
	if err == nil || !process.terminated {
		t.Fatalf("expected timeout and process termination, got err=%v terminated=%v", err, process.terminated)
	}
	if _, statErr := os.Stat(options.StateFile); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("state file should not exist: %v", statErr)
	}
}

func TestServiceStartReportsEarlyExit(t *testing.T) {
	dir := t.TempDir()
	process := &fakeProcess{pid: 42, executable: "/qodana/bin/idea", done: make(chan error, 1)}
	process.done <- errors.New("boom")
	service := Service{Launcher: fakeLauncher{process: process}, Processes: &fakeController{}, PollInterval: time.Millisecond}

	_, err := service.Start(context.Background(), testStartOptions(dir))
	if err == nil || !process.terminated {
		t.Fatalf("expected early-exit error and cleanup, got err=%v", err)
	}
}

func TestServiceStopIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	controller := &fakeController{}
	service := Service{Processes: controller, PollInterval: time.Millisecond}

	status, err := service.Stop(context.Background(), stateFile, time.Second)
	if err != nil || status.Status != "stopped" {
		t.Fatalf("unexpected first stop: status=%+v err=%v", status, err)
	}
	state := State{Version: StateVersion, PID: 42, Executable: "/qodana", ProjectDir: dir, StartedAt: time.Now()}
	if err := WriteState(stateFile, state); err != nil {
		t.Fatal(err)
	}
	controller.running = true
	controller.matches = true
	status, err = service.Stop(context.Background(), stateFile, time.Second)
	if err != nil || status.Status != "stopped" || !controller.terminated {
		t.Fatalf("unexpected running stop: status=%+v err=%v", status, err)
	}
}

func TestServiceRefusesReusedPID(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	state := State{Version: StateVersion, PID: 42, Executable: "/qodana", ProjectDir: dir, StartedAt: time.Now()}
	if err := WriteState(stateFile, state); err != nil {
		t.Fatal(err)
	}
	controller := &fakeController{running: true, matches: false}
	service := Service{Processes: controller}

	_, err := service.Stop(context.Background(), stateFile, time.Second)
	if err == nil || controller.terminated {
		t.Fatalf("expected identity error without termination, got err=%v", err)
	}
}

func testStartOptions(dir string) StartOptions {
	return StartOptions{
		ProjectDir:  dir,
		StateFile:   filepath.Join(dir, "state.json"),
		ReadyFile:   filepath.Join(dir, "ready.json"),
		LogFile:     filepath.Join(dir, "mcp.log"),
		WaitTimeout: time.Second,
	}
}
