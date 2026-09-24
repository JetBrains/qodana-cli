/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	foundationexec "github.com/JetBrains/qodana-cli/internal/foundation/exec"
	"github.com/JetBrains/qodana-cli/internal/tooling"
	"github.com/spf13/cobra"
)

// runEdictJVM delegates to the embedded Kotlin application using the CLI's bundled
// runtime. In particular, stdout must remain an unmodified MCP protocol stream.
func runEdictJVM(command *cobra.Command, args ...string) error {
	cache, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("resolve Edict tooling cache: %w", err)
	}
	cache = filepath.Join(cache, "JetBrains", "Qodana", "edict")
	ctx, stop := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	process := exec.CommandContext(ctx, tooling.GetQodanaJBRPath(cache),
		append([]string{"-jar", tooling.EdictCli.GetLibPath(cache)}, args...)...)
	process.Stdout, process.Stderr = command.OutOrStdout(), command.ErrOrStderr()
	process.WaitDelay = 5 * time.Second
	process.Cancel = func() error { return foundationexec.RequestTermination(process.Process) }

	input := command.InOrStdin()
	var stdin io.WriteCloser
	if file, ok := input.(*os.File); ok {
		process.Stdin = file
	} else {
		// Own the pump instead of exec.Cmd's stdin copier: Wait must finish when the
		// JVM exits even if the host has left a transport's Read blocked indefinitely.
		stdin, err = process.StdinPipe()
		if err != nil {
			return err
		}
		defer stdin.Close()
		if closer, ok := input.(io.Closer); ok {
			defer closer.Close()
			process.Cancel = func() error {
				_ = closer.Close()
				return foundationexec.RequestTermination(process.Process)
			}
		}
	}
	if err := process.Start(); err != nil {
		return fmt.Errorf("start Edict JVM: %w", err)
	}
	if stdin != nil {
		go func() {
			_, _ = io.Copy(stdin, input)
			_ = stdin.Close()
		}()
	}
	err = process.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("Edict JVM: %w", err)
	}
	return nil
}

func codexSkillsDirectory(project bool, projectDir string) (string, error) {
	if project {
		absolute, err := filepath.Abs(projectDir)
		return filepath.Join(absolute, ".codex", "skills"), err
	}
	if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" {
		return filepath.Join(codexHome, "skills"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".codex", "skills"), nil
}
