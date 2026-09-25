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
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	edictmcp "github.com/JetBrains/qodana-cli/edict/mcp"
	"github.com/JetBrains/qodana-cli/internal/core"
	"github.com/JetBrains/qodana-cli/internal/core/corescan"
	"github.com/JetBrains/qodana-cli/internal/core/startup"
	foundationexec "github.com/JetBrains/qodana-cli/internal/foundation/exec"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	platformcmd "github.com/JetBrains/qodana-cli/internal/platform/cmd"
	"github.com/JetBrains/qodana-cli/internal/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/shirou/gopsutil/v3/process"
)

const mcpServerScript = "mcp-server"
const mcpIDEStarter = "mcpServer"

var streamableEndpointPattern = regexp.MustCompile(`Streamable HTTP endpoint:\s*(https?://[^\s\x1b]+)`)
var ideSSEEndpointPattern = regexp.MustCompile(`SSE URL:\s*(https?://[^\s\x1b]+)`)

type qodanaMCPLauncher struct{}

func (qodanaMCPLauncher) Launch(_ context.Context, request edictmcp.LaunchRequest) (edictmcp.Process, error) {
	if request.Starter == "" {
		request.Starter = mcpIDEStarter
	}
	if request.Starter != mcpServerScript && request.Starter != mcpIDEStarter {
		return nil, fmt.Errorf("unsupported MCP starter %q; use mcp-server or mcpServer", request.Starter)
	}
	if request.Port != 0 && request.Starter != mcpIDEStarter {
		return nil, fmt.Errorf("--port is not supported by the '%s' linter script; use --port=0", mcpServerScript)
	}

	qdenv.InitializeQodanaGlobalEnv(qdenv.EmptyEnvProvider())
	scanContext, runtimeDir, cleanup, err := prepareMCPScanContext(request)
	if err != nil {
		return nil, err
	}
	arguments := core.PrepareNativeServiceRunCommand(scanContext)
	if request.Starter == mcpIDEStarter {
		arguments = ideMCPArguments(scanContext.Prod().IdeScript, request)
	}
	return startMCPProcess(arguments, request, runtimeDir, cleanup)
}

func ideMCPArguments(executable string, request edictmcp.LaunchRequest) []string {
	args := []string{executable, mcpIDEStarter, "--project=" + request.ProjectDir, "--invocation-mode=direct",
		"--allowed-tools=generate_psi_tree,generate_inspection_kts_api,generate_inspection_kts_examples,run_inspection_kts"}
	// Omit port 0: the IDE starter selects a free port when no port is supplied.
	if request.Port != 0 {
		args = append(args, fmt.Sprintf("--port=%d", request.Port))
	}
	return args
}

func startMCPProcess(arguments []string, request edictmcp.LaunchRequest, runtimeDir string, cleanup func()) (edictmcp.Process, error) {
	logFile, err := os.OpenFile(request.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("opening MCP log file: %w", err)
	}
	reader, err := os.Open(request.LogFile)
	if err != nil {
		_ = logFile.Close()
		cleanup()
		return nil, fmt.Errorf("reading MCP log file: %w", err)
	}
	// Read only this launch's output, not an endpoint left by a previous process.
	if _, err := reader.Seek(0, io.SeekEnd); err != nil {
		_ = reader.Close()
		_ = logFile.Close()
		cleanup()
		return nil, err
	}
	command := exec.Command(arguments[0], arguments[1:]...)
	command.Dir = request.ProjectDir
	// The server outlives `mcp start`. Inherit the file directly so logging keeps
	// working after the CLI exits instead of leaving the server with broken pipes.
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Start(); err != nil {
		_ = reader.Close()
		_ = logFile.Close()
		cleanup()
		return nil, fmt.Errorf("launching %s: %w", arguments[0], err)
	}

	process := &qodanaMCPProcess{
		command: command, executable: command.Path, runtimeDir: runtimeDir,
		done: make(chan error, 1), exited: make(chan struct{}), log: logFile, cleanup: cleanup,
	}
	go detectMCPEndpoint(reader, request.ReadyFile, process.exited)
	go process.wait()
	return process, nil
}

func detectMCPEndpoint(reader *os.File, readyFile string, exited <-chan struct{}) {
	defer reader.Close()
	detector := &mcpEndpointDetector{log: io.Discard, readyFile: readyFile}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	buffer := make([]byte, 4096)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			_, _ = detector.Write(buffer[:n])
		}
		if detector.reported || (err != nil && err != io.EOF) {
			return
		}
		if err == io.EOF {
			select {
			case <-exited:
				return
			case <-ticker.C:
			}
		}
	}
}

func prepareMCPScanContext(request edictmcp.LaunchRequest) (corescan.Context, string, func(), error) {
	if request.Linter == "" && request.IDE == "" && os.Getenv(qdenv.QodanaDistEnv) == "" {
		return corescan.Context{}, "", func() {},
			fmt.Errorf("MCP startup requires --ide, --linter, or %s", qdenv.QodanaDistEnv)
	}
	commonCtx := computeNativeMCPContext(request)
	if commonCtx.Analyzer.IsContainer() {
		return corescan.Context{}, "", func() {},
			fmt.Errorf("the MCP server requires a native linter or IDE distribution")
	}
	configDir, cleanup, err := fs.CreateTempDir("qodana-mcp-config")
	if err != nil {
		return corescan.Context{}, "", func() {}, fmt.Errorf("creating MCP configuration directory: %w", err)
	}
	// Keep the service separate from scan caches and concurrent MCP instances.
	commonCtx.CacheDir = filepath.Join(configDir, "cache")
	commonCtx.ResultsDir = filepath.Join(configDir, "results")
	preparedHost := startup.PrepareNativeServiceHost(commonCtx)
	cliOptions := platformcmd.CliOptions{
		ProjectDir:   request.ProjectDir,
		Linter:       request.Linter,
		Ide:          request.IDE,
		WithinDocker: "false",
		Script:       mcpServerScript,
		Property:     request.Property,
	}
	scanContext := corescan.CreateContext(
		cliOptions,
		commonCtx,
		preparedHost,
		corescan.QodanaYamlConfig{},
		configDir,
	)
	return scanContext, configDir, cleanup, nil
}

func computeNativeMCPContext(request edictmcp.LaunchRequest) commoncontext.Context {
	return commoncontext.Compute(
		request.Linter, request.IDE, "", "false",
		"", "", "", qdenv.GetQodanaGlobalEnv(qdenv.QodanaToken), false,
		request.ProjectDir, request.ProjectDir, "",
	)
}

type mcpEndpointDetector struct {
	mu        sync.Mutex
	log       io.Writer
	readyFile string
	pending   []byte
	reported  bool
}

func (w *mcpEndpointDetector) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.log.Write(data); err != nil {
		return 0, err
	}
	w.pending = append(w.pending, data...)
	for {
		newline := bytes.IndexByte(w.pending, '\n')
		if newline < 0 {
			break
		}
		w.inspectLine(string(w.pending[:newline]))
		w.pending = w.pending[newline+1:]
	}
	return len(data), nil
}

func (w *mcpEndpointDetector) inspectLine(line string) {
	if w.reported {
		return
	}
	match := streamableEndpointPattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) != 2 {
		match = ideSSEEndpointPattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(match) == 2 {
			match[1] = strings.TrimSuffix(match[1], "/sse") + "/stream"
		}
	}
	if len(match) != 2 {
		return
	}
	if err := edictmcp.WriteReady(w.readyFile, edictmcp.Ready{Status: "ready", URL: match[1]}); err != nil {
		_, _ = fmt.Fprintf(w.log, "\nFailed to publish MCP readiness: %s\n", err)
		return
	}
	w.reported = true
}

type qodanaMCPProcess struct {
	command    *exec.Cmd
	executable string
	runtimeDir string
	done       chan error
	exited     chan struct{}
	log        io.Closer
	cleanup    func()
}

func (p *qodanaMCPProcess) PID() int32 { return int32(p.command.Process.Pid) }
func (p *qodanaMCPProcess) Executable() string {
	proc, err := process.NewProcess(p.PID())
	if err == nil {
		if executable, exeErr := proc.Exe(); exeErr == nil && executable != "" {
			return executable
		}
	}
	return p.executable
}
func (p *qodanaMCPProcess) RuntimeDir() string { return p.runtimeDir }
func (p *qodanaMCPProcess) Done() <-chan error { return p.done }

func (p *qodanaMCPProcess) Terminate() error {
	return foundationexec.RequestTermination(p.command.Process)
}

func (p *qodanaMCPProcess) Kill() error {
	return p.command.Process.Kill()
}

func (p *qodanaMCPProcess) wait() {
	err := p.command.Wait()
	close(p.exited)
	_ = p.log.Close()
	p.cleanup()
	p.done <- err
	close(p.done)
}
