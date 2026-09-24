/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"context"
	"errors"
	"strconv"

	"github.com/spf13/cobra"
)

// newEdictManagedMCPCommand serves state management independently of the
// IntelliJ inspection server launched by `edict mcp start`.
func newEdictManagedMCPCommand() *cobra.Command {
	var projectDir, stateDir, logDir string
	var httpPort int
	command := &cobra.Command{
		Use:   "edict-mcp",
		Short: "Serve managed Edict state and skill capabilities through the Kotlin JVM",
		Long: `Run the bundled Kotlin managed Edict server over stdin/stdout.
Use --http-port to serve Streamable HTTP on loopback instead.
The immutable skill policy is registered at startup. Only capability-bearing
managed tasks may mutate state, and child capabilities can only narrow access.
IntelliJ inspections use the separate 'edict mcp start' server.
GitHub and Space PR-review data is read directly by edict-mcp. Configure
GITHUB_TOKEN (or GH_TOKEN), SPACE_TOKEN, and optionally EDICT_GITHUB_API_URL or
EDICT_SPACE_URL in the server environment; provider tokens are never MCP arguments.

The first successful edict_plan_create call returns the manager capability.
It requires no token and can succeed only once per server lifetime. Keep the
returned token private to edict_manager; all other mutations require a token.
All stdout output is MCP protocol traffic. Readable activity is logged to
<project-dir>/log/edict/edict-mcp.log; protocol details are logged separately to
edict-mcp-system.log in the same directory. MCP activity also appears in
edict-agents.log alongside output supplied by the agent host. edict-agent-short.log
keeps the same agent messages with concise MCP summaries, omitting response bodies
and task prompts. Capability tokens are redacted.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			args := []string{"mcp", "--project-dir", projectDir}
			for _, option := range []struct{ name, value string }{
				{"state-dir", stateDir}, {"log-dir", logDir},
			} {
				if option.value != "" {
					args = append(args, "--"+option.name, option.value)
				}
			}
			if command.Flags().Changed("http-port") {
				if httpPort < 0 || httpPort > 65535 {
					return errors.New("http-port must be between 0 and 65535")
				}
				args = append(args, "--http-port", strconv.Itoa(httpPort))
			}
			err := runEdictJVM(command, args...)
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		},
	}
	command.Flags().StringVarP(&projectDir, "project-dir", "i", ".", "Project root used for the default state and log directories")
	command.Flags().StringVar(&stateDir, "state-dir", "", "Persisted Edict state directory (defaults to <project-dir>/.edict)")
	command.Flags().StringVar(&logDir, "log-dir", "", "Log root (defaults to <project-dir>/log; files are written under edict/)")
	command.Flags().IntVar(&httpPort, "http-port", 0, "Serve HTTP on loopback at this port (0 selects an available port)")
	return command
}
