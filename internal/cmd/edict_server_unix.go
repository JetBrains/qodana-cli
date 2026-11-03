//go:build !windows

/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"errors"
	"syscall"
)

// detachedProcessAttributes start the server in a session of its own: the terminal's or CI step's signals and its end
// do not reach it, and its pid names the process group of everything it starts.
func detachedProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

func killProcessSession(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
