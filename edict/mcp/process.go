/*
 * Copyright 2021-2024 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package mcp

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/shirou/gopsutil/v3/process"
)

type OSProcessController struct{}

func (OSProcessController) Matches(state State) (bool, bool, error) {
	proc, err := process.NewProcess(state.PID)
	if err != nil {
		if processIsGone(err) {
			return false, false, nil
		}
		return false, false, err
	}
	running, err := proc.IsRunning()
	if err != nil {
		if processIsGone(err) {
			return false, false, nil
		}
		return false, false, err
	}
	if !running {
		return false, false, nil
	}
	executable, err := proc.Exe()
	if err != nil {
		if processIsGone(err) {
			return false, false, nil
		}
		return true, false, err
	}
	return true, sameExecutable(executable, state.Executable), nil
}

func (OSProcessController) Terminate(pid int32) error {
	return signalProcess(pid, func(proc *process.Process) error { return proc.Terminate() })
}

func (OSProcessController) Kill(pid int32) error {
	return signalProcess(pid, func(proc *process.Process) error { return proc.Kill() })
}

func signalProcess(pid int32, signal func(*process.Process) error) error {
	proc, err := process.NewProcess(pid)
	if err != nil {
		if processIsGone(err) {
			return nil
		}
		return err
	}
	err = signal(proc)
	if processIsGone(err) {
		return nil
	}
	return err
}

func processIsGone(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, process.ErrorProcessNotRunning)
}

func sameExecutable(left, right string) bool {
	leftResolved, leftErr := filepath.EvalSymlinks(left)
	if leftErr == nil {
		left = leftResolved
	}
	rightResolved, rightErr := filepath.EvalSymlinks(right)
	if rightErr == nil {
		right = rightResolved
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
