/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

// newEdictManagedMCPCommand serves Edict state management and, on demand, IntelliJ inspections.
func newEdictManagedMCPCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "mcp",
		Short: "Manage the Kotlin Edict state server",
	}
	command.AddCommand(newEdictManagedMCPStartCommand())
	return command
}

func newEdictManagedMCPStartCommand() *cobra.Command {
	return newEdictManagedMCPStartCommandWithRunner(runEdictJVM)
}

func newEdictManagedMCPStartCommandWithRunner(run func(*cobra.Command, ...string) error) *cobra.Command {
	var projectDir, stateDir, sourceRepository, logDir, embeddingPython, ideDist, ideLinter string
	var ideProperties []string
	var ideWaitTimeout time.Duration
	var httpPort int
	command := &cobra.Command{
		Use:   "start",
		Short: "Serve managed Edict state and skill capabilities through the Kotlin JVM",
		Long: `Run the bundled Kotlin managed Edict server over stdin/stdout.
Use --http-port to serve Streamable HTTP on loopback instead.
The immutable skill policy is registered at startup. Only capability-bearing
managed tasks may mutate state, and child capabilities can only narrow access.
On the first inspection call the server starts a native IntelliJ MCP server for
--project-dir and stops it on exit; runs without inspections never start an IDE.
The IDE comes from --ide-dist, else --ide-linter, else QODANA_DIST; Docker linters
are not supported. IDE output is logged to edict/intellij-mcp.log under the log root.
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
and task prompts. Full records for each managed task/subagent are also written to
tasks/<full-task-id>.log. Capability tokens are redacted.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			// The JVM exits with this process, so a SIGKILL here cannot leave the server and its IDE running.
			args := []string{"mcp", "--project-dir", projectDir, "--parent-pid", strconv.Itoa(os.Getpid())}
			// The Kotlin server starts the IntelliJ MCP server through this same CLI binary, not whichever qodana is on PATH.
			if executable, err := os.Executable(); err == nil {
				args = append(args, "--qodana-executable", executable)
			}
			for _, option := range []struct{ name, value string }{
				{"state-dir", stateDir}, {"source-repository", sourceRepository},
				{"log-dir", logDir}, {"embedding-python", embeddingPython},
				{"ide-dist", ideDist}, {"ide-linter", ideLinter},
			} {
				if option.value != "" {
					args = append(args, "--"+option.name, option.value)
				}
			}
			for _, property := range ideProperties {
				args = append(args, "--ide-property", property)
			}
			if ideWaitTimeout != 0 {
				args = append(args, "--ide-wait-timeout", ideWaitTimeout.String())
			}
			if command.Flags().Changed("http-port") {
				if httpPort < 0 || httpPort > 65535 {
					return errors.New("http-port must be between 0 and 65535")
				}
				args = append(args, "--http-port", strconv.Itoa(httpPort))
			}
			err := run(command, args...)
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		},
	}
	command.Flags().StringVarP(&projectDir, "project-dir", "i", ".", "Project root used for the default state and log directories")
	command.Flags().StringVar(&stateDir, "state-dir", "", "Persisted Edict state directory (defaults to <project-dir>/.edict)")
	command.Flags().StringVar(&sourceRepository, "source-repository", "", "Reference Edict repository used to validate managed changes (defaults to <project-dir>)")
	command.Flags().StringVar(&logDir, "log-dir", "", "Log root (defaults to <project-dir>/log; files are written under edict/)")
	command.Flags().StringVar(&embeddingPython, "embedding-python", "", "Prepared Python interpreter with the bundled embedding dependencies")
	command.Flags().IntVar(&httpPort, "http-port", 0, "Serve HTTP on loopback at this port (0 selects an available port)")
	command.Flags().StringVar(&ideDist, "ide-dist", "", "Local IDE or Qodana linter distribution for inspections (overrides QODANA_DIST)")
	command.Flags().StringVar(&ideLinter, "ide-linter", "", "Qodana linter to download and run natively for inspections, e.g. qodana-jvm (overrides QODANA_DIST)")
	command.Flags().StringArrayVar(&ideProperties, "ide-property", nil, "Set a JVM property or option for the IDE")
	command.Flags().DurationVar(&ideWaitTimeout, "ide-wait-timeout", 0, "Maximum time to wait for the IDE MCP server to become ready (default 1m30s)")
	command.MarkFlagsMutuallyExclusive("ide-dist", "ide-linter")
	return command
}
