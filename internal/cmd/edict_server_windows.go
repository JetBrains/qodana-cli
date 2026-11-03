/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"os/exec"
	"strconv"
	"syscall"
)

// detachedProcess starts a process without the console of its parent.
const detachedProcess = 0x00000008

// detachedProcessAttributes start the server in a process group of its own, without the console it was started from.
func detachedProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess}
}

func killProcessSession(pid int) error {
	return exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}
