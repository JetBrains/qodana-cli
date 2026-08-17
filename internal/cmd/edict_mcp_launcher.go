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
	"github.com/JetBrains/qodana-cli/internal/core/startup"
	foundationexec "github.com/JetBrains/qodana-cli/internal/foundation/exec"
	"github.com/JetBrains/qodana-cli/internal/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
)

const mcpServerScript = "mcp-server"

var streamableEndpointPattern = regexp.MustCompile(`Streamable HTTP endpoint:\s*(https?://[^\s\x1b]+)`)

type qodanaMCPLauncher struct{}

func (qodanaMCPLauncher) Launch(_ context.Context, request edictmcp.LaunchRequest) (edictmcp.Process, error) {
	if request.Port != 0 {
		return nil, fmt.Errorf("--port is not supported by the '%s' linter script; use --port=0", mcpServerScript)
	}

	qdenv.InitializeQodanaGlobalEnv(qdenv.EmptyEnvProvider())
	commonCtx := computeNativeMCPContext(request)
	if commonCtx.Analyzer.IsContainer() {
		return nil, fmt.Errorf("the MCP server requires a native linter or IDE distribution")
	}
	preparedHost := startup.PrepareHost(commonCtx)
	arguments := mcpLinterArguments(preparedHost.Prod, request.ProjectDir, commonCtx.ResultsDir)

	logFile, err := os.OpenFile(request.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening MCP log file: %w", err)
	}
	detector := &mcpEndpointDetector{log: logFile, readyFile: request.ReadyFile}
	command := exec.Command(arguments[0], arguments[1:]...)
	command.Dir = request.ProjectDir
	command.Stdout = detector
	command.Stderr = detector
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("launching %s: %w", arguments[0], err)
	}

	process := &qodanaMCPProcess{
		command: command, executable: command.Path, done: make(chan error, 1), log: logFile,
	}
	go process.wait()
	return process, nil
}

func computeNativeMCPContext(request edictmcp.LaunchRequest) commoncontext.Context {
	commonCtx := commoncontext.Compute(
		request.Linter, request.IDE, "", "false",
		"", "", "", qdenv.GetQodanaGlobalEnv(qdenv.QodanaToken), false,
		request.ProjectDir, "", "",
	)
	if commonCtx.Analyzer.IsContainer() && commonCtx.Analyzer.GetLinter() != product.UnknownLinter {
		commonCtx = commoncontext.Compute(
			commonCtx.Analyzer.GetLinter().Name, "", "", "false",
			"", "", "", qdenv.GetQodanaGlobalEnv(qdenv.QodanaToken), false,
			request.ProjectDir, "", "",
		)
	}
	return commonCtx
}

func mcpLinterArguments(prod product.Product, projectDir, resultsDir string) []string {
	arguments := []string{prod.IdeScript}
	if !prod.Is242orNewer() {
		arguments = append(arguments, "inspect")
	}
	return append(arguments, "qodana", "--script", mcpServerScript, projectDir, resultsDir)
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
	done       chan error
	log        io.Closer
}

func (p *qodanaMCPProcess) PID() int32         { return int32(p.command.Process.Pid) }
func (p *qodanaMCPProcess) Executable() string { return p.executable }
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
	p.done <- err
	close(p.done)
}
