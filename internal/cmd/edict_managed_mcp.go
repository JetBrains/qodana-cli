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
	"github.com/spf13/pflag"
)

// newEdictManagedMCPCommand serves Edict state management and, on demand, IntelliJ inspections.
func newEdictManagedMCPCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "mcp",
		Short: "Manage the Kotlin Edict state server",
	}
	command.AddCommand(newEdictManagedMCPStartCommand(), newEdictManagedMCPStopCommand())
	return command
}

func newEdictManagedMCPStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the Edict server that `edict mcp start` started for the current directory",
		Long: `Stop the detached Edict server of the project in the current directory and the IDE it started.
The server gets a minute to stop its IDE; then everything it started is killed.
Succeeds when no server runs.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			files, err := newEdictServerFiles(".")
			if err != nil {
				return err
			}
			return stopEdictServer(files, edictServerStopTimeout, command.OutOrStdout())
		},
	}
}

func newEdictManagedMCPStartCommand() *cobra.Command {
	return newEdictManagedMCPStartCommandWithRunner(runEdictJVM)
}

func newEdictManagedMCPStartCommandWithRunner(run func(*cobra.Command, ...string) error) *cobra.Command {
	var configFile, ideDist, ideLinter string
	var ideProperties []string
	var ideWaitTimeout time.Duration
	var foreground bool
	command := &cobra.Command{
		Use:   "start",
		Short: "Serve managed Edict state and skill capabilities through the Kotlin JVM",
		Long: `Run the bundled Kotlin managed Edict server for the project in the current directory.
The server runs in the background: the command returns once it serves MCP, its output
goes to a server log in the user cache, and 'qodana edict mcp stop' stops it and its IDE.
With --foreground it serves in this process until interrupted instead.
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
GitHub and Space PR-review data is read directly by edict-mcp from edict.ci.url;
promotion PRs go to edict.promotion.url, defaulting to edict.ci.url. Each URL's
host selects the provider API, including GitHub Enterprise. Configure GITHUB_TOKEN
(or GH_TOKEN) or SPACE_TOKEN for every configured provider in the server
environment; provider tokens are never MCP arguments.

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
			if !foreground {
				return startDetached(command)
			}
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
	command.Flags().DurationVar(&ideWaitTimeout, "ide-wait-timeout", 0, "Maximum time to wait for the IDE MCP server to become ready (default 1h)")
	command.Flags().BoolVar(&foreground, "foreground", false, "Serve in this process until it is interrupted, instead of in the background")
	command.MarkFlagsMutuallyExclusive("ide-dist", "ide-linter")
	return command
}

// startDetached starts this command again with --foreground and the same flags, in the background.
func startDetached(command *cobra.Command) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	files, err := newEdictServerFiles(".")
	if err != nil {
		return err
	}
	return startDetachedEdictServer(executable, foregroundArguments(command.Flags()), files, command.OutOrStdout())
}

// foregroundArguments start the server in the foreground with every flag set on the command line, as given.
func foregroundArguments(flags *pflag.FlagSet) []string {
	arguments := []string{"edict", "mcp", "start", "--foreground"}
	flags.Visit(func(flag *pflag.Flag) {
		values := []string{flag.Value.String()}
		if slice, ok := flag.Value.(pflag.SliceValue); ok {
			values = slice.GetSlice()
		}
		for _, value := range values {
			arguments = append(arguments, "--"+flag.Name+"="+value)
		}
	})
	return arguments
}

func edictQodanaYamlArguments(projectDir, configFile string) []string {
	fullPath := qdyaml.GetLocalNotEffectiveQodanaYamlFullPath(projectDir, configFile)
	if fullPath == "" {
		return nil
	}
	return []string{"--qodana-yaml", fullPath}
}
