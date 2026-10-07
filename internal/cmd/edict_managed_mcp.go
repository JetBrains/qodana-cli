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

	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
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
	var configFile, ideDist, ideLinter string
	var ideProperties []string
	var ideWaitTimeout time.Duration
	command := &cobra.Command{
		Use:   "start",
		Short: "Serve managed Edict state and skill capabilities through the Kotlin JVM",
		Long: `Run the bundled Kotlin managed Edict server for the project in the current directory.
It serves Streamable HTTP on loopback at edict.mcpPort from ./qodana.yaml (or
./qodana.yml; default 27182), and fails if 'qodana edict install' did not configure
that port in ./.codex/config.toml or the port is busy.
Persisted state is read from edict.statePath in that configuration (default ./.edict).
The immutable skill policy is registered at startup. Only capability-bearing
managed tasks may mutate state, and child capabilities can only narrow access.
On the first inspection call the server starts a native IntelliJ MCP server for
the current directory and stops it on exit; runs without inspections never start an IDE.
The IDE comes from --ide-dist, else --ide-linter, else QODANA_DIST; Docker linters
are not supported. IDE output is logged to intellij-mcp.log in the run's log directory.
GitHub and Space PR-review data is read directly by edict-mcp. Configure
GITHUB_TOKEN (or GH_TOKEN), SPACE_TOKEN, and optionally EDICT_GITHUB_API_URL or
EDICT_SPACE_URL in the server environment; provider tokens are never MCP arguments.

The first successful edict_plan_create call returns the manager capability.
It requires no token and can succeed only once per server lifetime. Keep the
returned token private to edict_manager; all other mutations require a token.
Each run logs to ./log/process-log/<run-id>, where the run id is its start time.
Readable activity is logged to edict-mcp.log there; protocol details are
logged separately to edict-mcp-system.log in the same directory. MCP activity also
appears in edict-agents.log alongside output supplied by the agent host.
edict-agent-short.log keeps the same agent messages with concise MCP summaries,
omitting response bodies and task prompts. Full records for each managed
task/subagent are also written to tasks/<full-task-id>.log. Capability tokens are
redacted. Agent scratch lives in ./log/agent-work/<run-id>/scratch.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			// The JVM exits with this process, so a SIGKILL here cannot leave the server and its IDE running.
			args := []string{"mcp", "--parent-pid", strconv.Itoa(os.Getpid())}
			// The Kotlin server starts the IntelliJ MCP server through this same CLI binary, not whichever qodana is on PATH.
			if executable, err := os.Executable(); err == nil {
				args = append(args, "--qodana-executable", executable)
			}
			args = append(args, edictQodanaYamlArguments(".", configFile)...)
			for _, option := range []struct{ name, value string }{
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
			err := run(command, args...)
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		},
	}
	command.Flags().StringVar(&configFile, "config", "", "Qodana configuration file (defaults to qodana.yml or qodana.yaml in the current directory)")
	command.Flags().StringVar(&ideDist, "ide-dist", "", "Local IDE or Qodana linter distribution for inspections (overrides QODANA_DIST)")
	command.Flags().StringVar(&ideLinter, "ide-linter", "", "Qodana linter to download and run natively for inspections, e.g. qodana-jvm (overrides QODANA_DIST)")
	command.Flags().StringArrayVar(&ideProperties, "ide-property", nil, "Set a JVM property or option for the IDE")
	command.Flags().DurationVar(&ideWaitTimeout, "ide-wait-timeout", 0, "Maximum time to wait for the IDE MCP server to become ready (default 1m30s)")
	command.MarkFlagsMutuallyExclusive("ide-dist", "ide-linter")
	return command
}

func edictQodanaYamlArguments(projectDir, configFile string) []string {
	fullPath := qdyaml.GetLocalNotEffectiveQodanaYamlFullPath(projectDir, configFile)
	if fullPath == "" {
		return nil
	}
	return []string{"--qodana-yaml", fullPath}
}
