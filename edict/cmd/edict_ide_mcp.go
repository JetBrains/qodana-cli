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

// The IDE gets this long to exit after SIGTERM before it is killed.
const ideMCPStopTimeout = 10 * time.Second

var (
	streamableEndpointPattern = regexp.MustCompile(`Streamable HTTP endpoint:\s*(https?://[^\s\x1b]+)`)
	mcpAuthTokenFilePattern   = regexp.MustCompile(`Send the header IJ_MCP_AUTH_TOKEN with the token in ([^\x1b\r\n]+)`)
)

type ideMCPEndpoint struct {
	URL           string `json:"url"`
	AuthTokenFile string `json:"authTokenFile,omitempty"`
}

type ideMCPReady struct {
	ideMCPEndpoint
	Status string `json:"status"`
	PID    int    `json:"pid"`
}

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
Stdout receives one JSON readiness line with the URL, IDE PID, and the auth token file if announced.
The caller stops the IDE by pid if this helper is killed before it can.
IDE output is forwarded to stderr. The Kotlin Edict server runs this on its first inspection call.
The linter kind are defined by --linter, else QODANA_DIST.`,
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
	flags.StringVar(&options.Dist, "dist", "", "Local Qodana linter distribution (overrides QODANA_DIST)")
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
	// Large projects take tens of minutes to import, and the IDE serves MCP only afterwards.
	flags.DurationVar(&options.WaitTimeout, "wait-timeout", time.Hour, "Maximum time to wait for MCP readiness")
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
	return core.PrepareNativeServiceRunCommand(scanContext), cleanup, nil
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
	// exited reports why the IDE exited on its own; stopped only tells stopIDE that a requested stop is done.
	exited := make(chan error, 1)
	stopped := make(chan struct{})
	go func() {
		err := ide.Wait()
		_ = outputWriter.Close()
		exited <- err
		close(stopped)
	}()
	endpoints := make(chan ideMCPEndpoint, 1)
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
		case endpoint := <-endpoints:
			ready, endpoints = true, nil
			deadline.Stop()
			readiness := ideMCPReady{ideMCPEndpoint: endpoint, Status: "ready", PID: ide.Process.Pid}
			if err := json.NewEncoder(stdout).Encode(readiness); err != nil {
				return errors.Join(fmt.Errorf("publishing MCP readiness: %w", err), stopIDE(ide, stopped))
			}
		case err := <-exited:
			if err == nil {
				err = errors.New("process exited")
			}
			if ready {
				return fmt.Errorf("IntelliJ MCP server exited: %w", err)
			}
			return fmt.Errorf("IntelliJ MCP server exited before becoming ready (the linter must support the mcp-server script): %w", err)
		case <-deadline.C:
			return errors.Join(
				fmt.Errorf("IntelliJ MCP server did not become ready within %s", waitTimeout),
				stopIDE(ide, stopped),
			)
		case <-stdinClosed:
			return stopIDE(ide, stopped)
		case <-ctx.Done():
			return stopIDE(ide, stopped)
		}
	}
}

// stopIDE asks the IDE to terminate, kills it if it does not stop in time, and waits until it has exited. The exit
// status of an IDE that was asked to stop is not a failure.
func stopIDE(ide *exec.Cmd, stopped <-chan struct{}) error {
	if err := foundationexec.RequestTermination(ide.Process); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = ide.Process.Kill()
	}
	select {
	case <-stopped:
		return nil
	case <-time.After(ideMCPStopTimeout):
		if err := ide.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("killing IntelliJ MCP server: %w", err)
		}
		<-stopped
		return nil
	}
}

// forwardIDEOutput forwards the script output and publishes the endpoint after the startup banner.
func forwardIDEOutput(output io.Reader, log io.Writer, endpoints chan<- ideMCPEndpoint) {
	reader := bufio.NewReader(output)
	var endpoint ideMCPEndpoint
	reported := false
	for {
		line, err := reader.ReadString('\n')
		_, _ = io.WriteString(log, line)
		if !reported {
			if match := streamableEndpointPattern.FindStringSubmatch(line); match != nil {
				endpoint.URL = match[1]
			}
			if match := mcpAuthTokenFilePattern.FindStringSubmatch(line); match != nil {
				endpoint.AuthTokenFile = strings.TrimSpace(match[1])
			}
			if endpoint.URL != "" && strings.Contains(line, "Terminate the process to stop the server and close the project") {
				reported = true
				endpoints <- endpoint
			}
		}
		if err != nil {
			return
		}
	}
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
		Script:       "mcp-server",
		Property:     ideMCPProperties(options.Property),
	}
	scanContext := corescan.CreateContext(cliOptions, commonCtx, preparedHost, corescan.QodanaYamlConfig{}, configDir)
	return scanContext, cleanup, nil
}

// ideMCPDefaultProperties configure the IDE for an Edict session on any project.
var ideMCPDefaultProperties = []string{
	// 70% of the memory the JVM detects, cgroup limits included; with it, the launcher drops the distribution's -Xmx.
	// The rest holds the build tool's daemon during the import, the Kotlin daemon and the agents. An explicit -Xmx wins.
	"-XX:MaxRAMPercentage=70",
	// Avoid repeated JSON schema lookups while the batch engine discovers injected code.
	"idea.batch.inspections.inspect.injected.psi=false",
	// Background work that a session without an editor does not need, as in Qodana's rules extraction builds.
	"idea.enable.duplicates.online.calculation=false",
	"vfs.health-check.enabled=false",
	// The Kotlin daemon that compiles inspection scripts would otherwise inherit the IDE's heap limit.
	"kotlin.daemon.jvm.options=-Xmx2g",
	// The Gradle daemon that imported the project stays idle for three hours by default, holding its heap while agents
	// run inspections. The IDE's own JVM properties override the project's gradle.properties for the daemons it starts.
	"org.gradle.daemon.idletimeout=60000",
}

// ideMCPProperties are the default properties, except those the caller sets, followed by the caller's.
func ideMCPProperties(properties []string) []string {
	overridden := map[string]bool{}
	for _, property := range properties {
		overridden[ideMCPPropertyKey(property)] = true
	}
	var result []string
	for _, property := range ideMCPDefaultProperties {
		if !overridden[ideMCPPropertyKey(property)] {
			result = append(result, property)
		}
	}
	return append(result, properties...)
}

// ideMCPPropertyKey is the system property or JVM option a property sets: the part before "=", without "-D".
func ideMCPPropertyKey(property string) string {
	key, _, _ := strings.Cut(property, "=")
	return strings.TrimPrefix(key, "-D")
}

func computeNativeMCPContext(options ideMCPOptions) commoncontext.Context {
	return commoncontext.Compute(
		options.Linter, "", "", "false",
		"", "", "", qdenv.GetQodanaGlobalEnv(qdenv.QodanaToken), false,
		options.ProjectDir, options.ProjectDir, "",
	)
}
