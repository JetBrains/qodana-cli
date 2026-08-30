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
	"regexp"
	"strings"
	"sync"

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

var streamableEndpointPattern = regexp.MustCompile(`Streamable HTTP endpoint:\s*(https?://[^\s\x1b]+)`)

type qodanaMCPLauncher struct{}

func (qodanaMCPLauncher) Launch(_ context.Context, request edictmcp.LaunchRequest) (edictmcp.Process, error) {
	if request.Port != 0 {
		return nil, fmt.Errorf("--port is not supported by the '%s' linter script; use --port=0", mcpServerScript)
	}

	qdenv.InitializeQodanaGlobalEnv(qdenv.EmptyEnvProvider())
	scanContext, runtimeDir, cleanup, err := prepareMCPScanContext(request)
	if err != nil {
		return nil, err
	}
	arguments := core.PrepareNativeServiceRunCommand(scanContext)

	logFile, err := os.OpenFile(request.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("opening MCP log file: %w", err)
	}
	detector := &mcpEndpointDetector{log: logFile, readyFile: request.ReadyFile}
	command := exec.Command(arguments[0], arguments[1:]...)
	command.Dir = request.ProjectDir
	command.Stdout = detector
	command.Stderr = detector
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		cleanup()
		return nil, fmt.Errorf("launching %s: %w", arguments[0], err)
	}

	process := &qodanaMCPProcess{
		command: command, executable: command.Path, runtimeDir: runtimeDir,
		done: make(chan error, 1), log: logFile, cleanup: cleanup,
	}
	go process.wait()
	return process, nil
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
	preparedHost := startup.PrepareNativeServiceHost(commonCtx)
	configDir, cleanup, err := fs.CreateTempDir("qodana-mcp-config")
	if err != nil {
		return corescan.Context{}, "", func() {}, fmt.Errorf("creating MCP configuration directory: %w", err)
	}
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
	_ = p.log.Close()
	p.cleanup()
	p.done <- err
	close(p.done)
}
