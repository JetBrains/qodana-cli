/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/JetBrains/qodana-cli/internal/core"
	"github.com/JetBrains/qodana-cli/internal/core/corescan"
	"github.com/JetBrains/qodana-cli/internal/core/startup"
	foundationexec "github.com/JetBrains/qodana-cli/internal/foundation/exec"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	platformcmd "github.com/JetBrains/qodana-cli/internal/platform/cmd"
	"github.com/JetBrains/qodana-cli/internal/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/spf13/cobra"
)

const (
	// Qodana's native service script; its name selects the MCP service properties.
	mcpServerScript = "mcp-server"
	mcpIDEStarter   = "mcpServer"
	// The IDE gets this long to exit after SIGTERM before it is killed.
	ideMCPStopTimeout = 10 * time.Second
)

// ideMCPTools are the IntelliJ tools the Kotlin Edict server calls or forwards; keep both sides in sync.
var ideMCPTools = []string{
	"generate_psi_tree",
	"generate_inspection_kts_api",
	"generate_inspection_kts_examples",
	"run_inspection_kts",
	"compile_inspection_kts",
	"run_inspection_kts_examples",
	"run_inspection_kts_project",
}

var (
	streamableEndpointPattern = regexp.MustCompile(`Streamable HTTP endpoint:\s*(https?://[^\s\x1b]+)`)
	ideSSEEndpointPattern     = regexp.MustCompile(`SSE URL:\s*(https?://[^\s\x1b]+)`)
)

// ideMCPOptions select a native IDE: a local distribution, a Qodana linter the CLI downloads, or QODANA_DIST.
// Docker is not supported, since the IDE must read the host paths the Kotlin server passes to it.
type ideMCPOptions struct {
	ProjectDir  string
	Dist        string
	Linter      string
	Property    []string
	ResultsDir  string
	WaitTimeout time.Duration
}

// newEdictIDEMCPCommand runs one IntelliJ MCP server for the lifetime of its caller, the Kotlin Edict server.
func newEdictIDEMCPCommand() *cobra.Command {
	options := ideMCPOptions{}
	command := &cobra.Command{
		Use:   "ide-mcp",
		Short: "Run the IntelliJ MCP server for one Edict session (internal)",
		Long: `Start the native IDE MCP server and keep it running until stdin closes or SIGTERM/SIGINT arrives.
Stdout receives exactly one line, {"status":"ready","url":...,"pid":...}, once the IDE serves MCP.
The caller stops the IDE by pid if this helper is killed before it can.
IDE output is forwarded to stderr. The Kotlin Edict server runs this on its first inspection call.
The IDE comes from --dist, else --linter, else QODANA_DIST; Docker is not supported.`,
		Hidden:       true,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		// The root command prints the error once; the Kotlin server shows it from this command's stderr.
		SilenceErrors: true,
		RunE: func(command *cobra.Command, _ []string) error {
			return runIDEMCP(
				command.Context(),
				options,
				command.InOrStdin(),
				command.OutOrStdout(),
				command.ErrOrStderr(),
			)
		},
	}
	flags := command.Flags()
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the project")
	flags.StringVar(&options.Dist, "dist", "", "Local IDE or Qodana linter distribution (overrides QODANA_DIST)")
	flags.StringVar(
		&options.Linter,
		"linter",
		"",
		"Qodana linter to download and run natively, e.g. qodana-jvm (overrides QODANA_DIST)",
	)
	flags.StringArrayVar(&options.Property, "property", nil, "Set a JVM property or option for the IDE")
	flags.StringVar(
		&options.ResultsDir,
		"results-dir",
		"",
		"Directory for IDE results and logs, kept after exit (default: a temporary directory removed on exit)",
	)
	flags.DurationVar(&options.WaitTimeout, "wait-timeout", 90*time.Second, "Maximum time to wait for MCP readiness")
	command.MarkFlagsMutuallyExclusive("dist", "linter")
	return command
}

func runIDEMCP(ctx context.Context, options ideMCPOptions, stdin io.Reader, stdout, stderr io.Writer) error {
	if options.WaitTimeout <= 0 {
		return errors.New("wait timeout must be positive")
	}
	projectDir, err := fs.Canonical(options.ProjectDir)
	if err != nil {
		return fmt.Errorf("resolving project directory: %w", err)
	}
	options.ProjectDir = projectDir
	if options.ResultsDir != "" {
		// The IDE runs in the project directory, so a relative path must not be resolved from there.
		if options.ResultsDir, err = filepath.Abs(options.ResultsDir); err != nil {
			return fmt.Errorf("resolving results directory: %w", err)
		}
	}
	if err := selectIDEDistribution(options); err != nil {
		return err
	}

	// Qodana's host preparation prints notices, such as the EAP token warning, to os.Stdout. Send them to stderr:
	// stdout carries only the readiness line, and the caller already holds the real stdout writer.
	processStdout := os.Stdout
	os.Stdout = os.Stderr
	arguments, cleanup, err := ideMCPCommand(options)
	os.Stdout = processStdout
	if err != nil {
		return err
	}
	defer cleanup()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveIDEMCP(ctx, arguments, projectDir, options.WaitTimeout, stdin, stdout, stderr)
}

// selectIDEDistribution makes the chosen IDE authoritative for this process and the IDE it starts:
// Qodana's analyzer lookup otherwise lets QODANA_DIST silently override an explicit linter.
func selectIDEDistribution(options ideMCPOptions) error {
	dist := options.Dist
	switch {
	case dist != "": // --dist wins over --linter and QODANA_DIST; it is resolved below.
	case options.Linter != "":
		return os.Unsetenv(qdenv.QodanaDistEnv)
	case os.Getenv(qdenv.QodanaDistEnv) != "":
		dist = os.Getenv(qdenv.QodanaDistEnv)
	default:
		return fmt.Errorf("IntelliJ for Edict inspections requires --dist, --linter, or %s", qdenv.QodanaDistEnv)
	}
	// The IDE runs in the project directory, so a relative distribution path must not be resolved from there.
	absolute, err := filepath.Abs(dist)
	if err != nil {
		return fmt.Errorf("resolving IDE distribution %s: %w", dist, err)
	}
	return os.Setenv(qdenv.QodanaDistEnv, absolute)
}

// ideMCPCommand prepares the native IDE host and returns the MCP server command line.
func ideMCPCommand(options ideMCPOptions) ([]string, func(), error) {
	qdenv.InitializeQodanaGlobalEnv(qdenv.EmptyEnvProvider())
	scanContext, cleanup, err := prepareMCPScanContext(options)
	if err != nil {
		return nil, cleanup, err
	}
	// Only for its side effects: it writes the vmoptions file and exports its environment variable for the IDE.
	_ = core.PrepareNativeServiceRunCommand(scanContext)
	return ideMCPArguments(scanContext.Prod().IdeScript, options.ProjectDir), cleanup, nil
}

// ideMCPArguments starts the IDE's headless MCP server; without --port it selects a free port.
func ideMCPArguments(executable string, projectDir string) []string {
	return []string{
		executable, mcpIDEStarter, "--project=" + projectDir, "--invocation-mode=direct",
		"--allowed-tools=" + strings.Join(ideMCPTools, ","),
	}
}

// serveIDEMCP runs the IDE until stdin closes, ctx ends, or the IDE exits, and reports readiness once.
func serveIDEMCP(
	ctx context.Context,
	arguments []string,
	dir string,
	waitTimeout time.Duration,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	output, outputWriter := io.Pipe()
	ide := exec.Command(arguments[0], arguments[1:]...)
	ide.Dir = dir
	ide.Stdout, ide.Stderr = outputWriter, outputWriter
	if err := ide.Start(); err != nil {
		return fmt.Errorf("launching %s: %w", arguments[0], err)
	}
	exited := make(chan error, 1)
	go func() {
		err := ide.Wait()
		_ = outputWriter.Close()
		exited <- err
	}()
	endpoints := make(chan string, 1)
	go forwardIDEOutput(output, stderr, endpoints)
	stdinClosed := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, stdin)
		close(stdinClosed)
	}()

	deadline := time.NewTimer(waitTimeout)
	defer deadline.Stop()
	ready := false
	for {
		select {
		case url := <-endpoints:
			ready, endpoints = true, nil
			deadline.Stop()
			readiness := map[string]any{"status": "ready", "url": url, "pid": ide.Process.Pid}
			if err := json.NewEncoder(stdout).Encode(readiness); err != nil {
				return errors.Join(fmt.Errorf("publishing MCP readiness: %w", err), stopIDE(ide, exited))
			}
		case err := <-exited:
			if err == nil {
				err = errors.New("process exited")
			}
			if ready {
				return fmt.Errorf("IntelliJ MCP server exited: %w", err)
			}
			return fmt.Errorf("IntelliJ MCP server exited before becoming ready: %w", err)
		case <-deadline.C:
			return errors.Join(
				fmt.Errorf("IntelliJ MCP server did not become ready within %s", waitTimeout),
				stopIDE(ide, exited),
			)
		case <-stdinClosed:
			return stopIDE(ide, exited)
		case <-ctx.Done():
			return stopIDE(ide, exited)
		}
	}
}

func stopIDE(ide *exec.Cmd, exited <-chan error) error {
	if err := foundationexec.RequestTermination(ide.Process); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = ide.Process.Kill()
	}
	select {
	case <-exited:
		return nil
	case <-time.After(ideMCPStopTimeout):
		if err := ide.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("killing IntelliJ MCP server: %w", err)
		}
		<-exited
		return nil
	}
}

// forwardIDEOutput copies IDE output line by line to log and sends the first MCP endpoint it announces.
func forwardIDEOutput(output io.Reader, log io.Writer, endpoints chan<- string) {
	reader := bufio.NewReader(output)
	reported := false
	for {
		line, err := reader.ReadString('\n')
		_, _ = io.WriteString(log, line)
		if url, ok := mcpEndpoint(line); ok && !reported {
			reported = true
			endpoints <- url
		}
		if err != nil {
			return
		}
	}
}

func mcpEndpoint(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if match := streamableEndpointPattern.FindStringSubmatch(line); match != nil {
		return match[1], true
	}
	// IDE builds that only announce SSE also serve Streamable HTTP on the sibling /stream path.
	if match := ideSSEEndpointPattern.FindStringSubmatch(line); match != nil {
		return strings.TrimSuffix(match[1], "/sse") + "/stream", true
	}
	return "", false
}

func prepareMCPScanContext(options ideMCPOptions) (corescan.Context, func(), error) {
	noCleanup := func() {}
	commonCtx := computeNativeMCPContext(options)
	if commonCtx.Analyzer.IsContainer() {
		return corescan.Context{}, noCleanup, errors.New("edict inspections require a native IDE; Docker linters are not supported")
	}
	configDir, cleanup, err := fs.CreateTempDir("qodana-mcp-config")
	if err != nil {
		return corescan.Context{}, noCleanup, fmt.Errorf("creating MCP configuration directory: %w", err)
	}
	// Keep the service separate from scan caches and concurrent MCP instances.
	commonCtx.CacheDir = filepath.Join(configDir, "cache")
	commonCtx.ResultsDir = options.ResultsDir
	if commonCtx.ResultsDir == "" {
		commonCtx.ResultsDir = filepath.Join(configDir, "results")
	}
	preparedHost := startup.PrepareNativeServiceHost(commonCtx)
	cliOptions := platformcmd.CliOptions{
		ProjectDir:   options.ProjectDir,
		Linter:       options.Linter,
		WithinDocker: "false",
		Script:       mcpServerScript,
		Property:     options.Property,
	}
	scanContext := corescan.CreateContext(cliOptions, commonCtx, preparedHost, corescan.QodanaYamlConfig{}, configDir)
	return scanContext, cleanup, nil
}

func computeNativeMCPContext(options ideMCPOptions) commoncontext.Context {
	return commoncontext.Compute(
		options.Linter, "", "", "false",
		"", "", "", qdenv.GetQodanaGlobalEnv(qdenv.QodanaToken), false,
		options.ProjectDir, options.ProjectDir, "",
	)
}
