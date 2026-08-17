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
		if errors.Is(err, os.ErrNotExist) {
			return false, false, nil
		}
		return false, false, err
	}
	running, err := proc.IsRunning()
	if err != nil || !running {
		return running, false, err
	}
	executable, err := proc.Exe()
	if err != nil {
		return true, false, err
	}
	return true, sameExecutable(executable, state.Executable), nil
}

func (OSProcessController) Terminate(pid int32) error {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return err
	}
	return proc.Terminate()
}

func (OSProcessController) Kill(pid int32) error {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
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
