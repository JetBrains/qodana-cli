/*
 * Copyright 2021-2024 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	edictmcp "github.com/JetBrains/qodana-cli/edict/mcp"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/spf13/cobra"
)

const (
	mcpOutputTabular = "tabular"
	mcpOutputJSON    = "json"
)

func newEdictMCPCommand() *cobra.Command {
	service := edictmcp.Service{
		Launcher:  qodanaMCPLauncher{},
		Processes: edictmcp.OSProcessController{},
	}
	return newEdictMCPCommandWithService(service)
}

func newEdictMCPCommandWithService(service edictmcp.Service) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage the Edict MCP server",
	}
	cmd.AddCommand(
		newEdictMCPStartCommand(service),
		newEdictMCPStatusCommand(service),
		newEdictMCPStopCommand(service),
	)
	return cmd
}

type edictMCPStartOptions struct {
	ProjectDir  string
	StateFile   string
	ReadyFile   string
	LogFile     string
	Linter      string
	IDE         string
	Property    []string
	Port        int
	WaitTimeout time.Duration
}

func newEdictMCPStartCommand(service edictmcp.Service) *cobra.Command {
	options := &edictMCPStartOptions{}
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the MCP server and wait until it is ready",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectDir, paths, err := resolveEdictMCPPaths(
				options.ProjectDir,
				options.StateFile,
				options.ReadyFile,
				options.LogFile,
			)
			if err != nil {
				return err
			}
			state, err := service.Start(
				cmd.Context(), edictmcp.StartOptions{
					ProjectDir: projectDir, StateFile: paths.state, ReadyFile: paths.ready, LogFile: paths.log,
					Linter: options.Linter, IDE: options.IDE, Property: options.Property, Port: options.Port, WaitTimeout: options.WaitTimeout,
				},
			)
			if err != nil {
				return err
			}
			return writeJSON(
				cmd, struct {
					Status    string `json:"status"`
					URL       string `json:"url"`
					StateFile string `json:"stateFile"`
				}{Status: "ready", URL: state.URL, StateFile: paths.state},
			)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the project")
	flags.StringVarP(&options.Linter, "linter", "l", "", "Qodana linter to run")
	flags.StringVar(
		&options.IDE,
		"ide",
		os.Getenv(qdenv.QodanaDistEnv),
		"Native IDE product code or path to a local IDE distribution",
	)
	flags.IntVar(&options.Port, "port", 0, "MCP server port; 0 selects an available port")
	flags.StringArrayVar(&options.Property, "property", nil, "Set a JVM property or option for the MCP server")
	flags.DurationVar(&options.WaitTimeout, "wait-timeout", 90*time.Second, "Maximum time to wait for MCP readiness")
	flags.StringVar(&options.StateFile, "state-file", "", "Lifecycle state file (defaults to the Qodana user cache)")
	flags.StringVar(&options.ReadyFile, "ready-file", "", "Readiness file written by the linter")
	flags.StringVar(&options.LogFile, "log-file", "", "MCP server log file")
	cmd.MarkFlagsMutuallyExclusive("linter", "ide")
	return cmd
}

type edictMCPStateOptions struct {
	ProjectDir string
	StateFile  string
	Timeout    time.Duration
	Output     string
}

func newEdictMCPStatusCommand(service edictmcp.Service) *cobra.Command {
	options := &edictMCPStateOptions{}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the MCP server status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, paths, err := resolveEdictMCPPaths(options.ProjectDir, options.StateFile, "", "")
			if err != nil {
				return err
			}
			status, err := service.Status(paths.state)
			if err != nil {
				return err
			}
			return writeMCPStatus(cmd, status, options.Output)
		},
	}
	addEdictMCPStateFlags(cmd, options, false)
	cmd.Flags().StringVarP(
		&options.Output,
		"output",
		"o",
		mcpOutputTabular,
		"Output format, can be tabular or json",
	)
	return cmd
}

func newEdictMCPStopCommand(service edictmcp.Service) *cobra.Command {
	options := &edictMCPStateOptions{}
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the MCP server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, paths, err := resolveEdictMCPPaths(options.ProjectDir, options.StateFile, "", "")
			if err != nil {
				return err
			}
			status, err := service.Stop(cmd.Context(), paths.state, options.Timeout)
			if err != nil {
				return err
			}
			return writeJSON(cmd, status)
		},
	}
	addEdictMCPStateFlags(cmd, options, true)
	return cmd
}

func addEdictMCPStateFlags(cmd *cobra.Command, options *edictMCPStateOptions, includeTimeout bool) {
	cmd.Flags().StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the project")
	cmd.Flags().StringVar(
		&options.StateFile,
		"state-file",
		"",
		"Lifecycle state file (defaults to the Qodana user cache)",
	)
	if includeTimeout {
		cmd.Flags().DurationVar(
			&options.Timeout,
			"wait-timeout",
			10*time.Second,
			"Time to wait before force-stopping MCP",
		)
	}
}

type edictMCPPaths struct {
	state string
	ready string
	log   string
}

func resolveEdictMCPPaths(projectDir, stateFile, readyFile, logFile string) (string, edictMCPPaths, error) {
	canonicalProject, err := fs.Canonical(projectDir)
	if err != nil {
		return "", edictMCPPaths{}, fmt.Errorf("resolving project directory: %w", err)
	}
	if stateFile == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return "", edictMCPPaths{}, fmt.Errorf("resolving user cache directory: %w", err)
		}
		digest := sha256.Sum256([]byte(canonicalProject))
		projectID := hex.EncodeToString(digest[:8])
		stateFile = filepath.Join(cacheDir, "JetBrains", "Qodana", "edict", "mcp-"+projectID+".json")
	}
	stateFile, err = filepath.Abs(stateFile)
	if err != nil {
		return "", edictMCPPaths{}, fmt.Errorf("resolving MCP state file: %w", err)
	}
	base := stateFile[:len(stateFile)-len(filepath.Ext(stateFile))]
	if readyFile == "" {
		readyFile = base + ".ready.json"
	}
	if logFile == "" {
		logFile = base + ".log"
	}
	readyFile, err = filepath.Abs(readyFile)
	if err != nil {
		return "", edictMCPPaths{}, err
	}
	logFile, err = filepath.Abs(logFile)
	if err != nil {
		return "", edictMCPPaths{}, err
	}
	return canonicalProject, edictMCPPaths{state: stateFile, ready: readyFile, log: logFile}, nil
}

func writeJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func writeMCPStatus(cmd *cobra.Command, status edictmcp.StatusResult, output string) error {
	switch output {
	case mcpOutputTabular:
		return writeMCPStatusTable(cmd.OutOrStdout(), status)
	case mcpOutputJSON:
		return writeJSON(cmd, status)
	default:
		return fmt.Errorf("unknown output format %q; use tabular or json", output)
	}
}

func writeMCPStatusTable(output io.Writer, status edictmcp.StatusResult) error {
	pid := ""
	if status.PID > 0 {
		pid = fmt.Sprint(status.PID)
	}
	started := ""
	if !status.StartedAt.IsZero() {
		started = status.StartedAt.Local().Format(time.RFC3339)
	}
	linterLabel, linter := "Linter", status.Linter
	if linter == "" {
		linterLabel, linter = "IDE", status.IDE
	}

	rows := []struct {
		label string
		value string
	}{
		{"Status", printableMCPStatus(status.Status)},
		{"Endpoint", status.URL},
		{"PID", pid},
		{"Project", status.ProjectDir},
		{"Started", started},
		{linterLabel, linter},
		{"Log", status.LogFile},
		{"State", status.StateFile},
	}

	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "Edict MCP server"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(writer); err != nil {
		return err
	}
	for _, row := range rows {
		if row.value == "" {
			continue
		}
		if _, err := fmt.Fprintf(writer, "%s:\t%s\n", row.label, row.value); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func printableMCPStatus(status string) string {
	switch status {
	case "running":
		return "✓ running"
	case "stopped":
		return "stopped"
	case "stale":
		return "! stale (process is no longer running)"
	case "pid-reused":
		return "! PID reused (state does not match process)"
	default:
		return status
	}
}
