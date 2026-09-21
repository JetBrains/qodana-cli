/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/JetBrains/qodana-cli/edict/managed"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// newEdictManagedMCPCommand serves state management independently of the
// IntelliJ inspection server launched by `edict mcp start`.
func newEdictManagedMCPCommand() *cobra.Command {
	var projectDir, stateDir string
	command := &cobra.Command{
		Use:   "edict-mcp",
		Short: "Serve managed Edict state and skill capabilities over MCP stdio",
		Long: `Run the Qodana CLI's managed Edict state server over stdin/stdout.
The immutable skill policy is registered at startup. Only capability-bearing
managed tasks may mutate state, and child capabilities can only narrow access.
IntelliJ inspections use the separate 'edict mcp start' server.

The first successful edict_plan_create call returns the manager capability.
It requires no token and can succeed only once per server lifetime. Keep the
returned token private to edict_manager; all other mutations require a token.
All stdout output is MCP protocol traffic. Readable activity is logged to
<project-dir>/log/edict/edict-mcp.log; protocol details are logged separately to
edict-mcp-system.log in the same directory. MCP activity also appears in
edict-agents.log alongside output supplied by the agent host. Capability tokens are redacted.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			logs, err := managed.OpenLogs(filepath.Join(projectDir, "log"))
			if err != nil {
				return err
			}
			defer logs.Close()
			if stateDir == "" {
				stateDir = filepath.Join(projectDir, ".edict")
			}
			store, err := managed.NewStore(stateDir)
			if err != nil {
				return err
			}
			defer store.Close()
			ctx, stop := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			reader, ok := command.InOrStdin().(io.ReadCloser)
			if !ok {
				reader = io.NopCloser(command.InOrStdin())
			}
			err = managed.NewServer(store, logs.Activity, logs.System, managed.NewAgentLogger(store, logs.Agents)).Run(ctx, &mcp.IOTransport{
				Reader: reader,
				Writer: managedMCPWriter{command.OutOrStdout()},
			})
			if errors.Is(err, context.Canceled) && ctx.Err() != nil {
				return nil
			}
			return err
		},
	}
	command.Flags().StringVarP(&projectDir, "project-dir", "i", ".", "Project root used for the default state and log directories")
	command.Flags().StringVar(&stateDir, "state-dir", "", "Persisted Edict state directory (defaults to <project-dir>/.edict)")
	return command
}

type managedMCPWriter struct{ io.Writer }

func (managedMCPWriter) Close() error { return nil }
