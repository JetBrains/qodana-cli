// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

// Logs keeps tool activity, protocol diagnostics, and runtime agent output separate.
type Logs struct {
	Activity    *os.File
	System      *os.File
	Agents      *os.File
	AgentsShort *os.File
}

func (l *Logs) Close() error {
	return errors.Join(l.Activity.Close(), l.System.Close(), l.Agents.Close(), l.AgentsShort.Close())
}

// OpenLogs appends managed logs below the caller's log directory. The caller must
// close them after all server sessions have stopped.
func OpenLogs(logDir string) (*Logs, error) {
	if strings.TrimSpace(logDir) == "" {
		return nil, fmt.Errorf("a log directory is required")
	}
	directory := filepath.Join(logDir, "edict")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("creating Edict log directory: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(directory, "edict-mcp.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening Edict MCP log: %w", err)
	}
	system, err := os.OpenFile(filepath.Join(directory, "edict-mcp-system.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("opening Edict MCP system log: %w", err)
	}
	agents, err := os.OpenFile(filepath.Join(directory, "edict-agents.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		_ = file.Close()
		_ = system.Close()
		return nil, fmt.Errorf("opening Edict agent output log: %w", err)
	}
	short, err := os.OpenFile(filepath.Join(directory, "edict-agent-short.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		_ = file.Close()
		_ = system.Close()
		_ = agents.Close()
		return nil, fmt.Errorf("opening Edict short agent output log: %w", err)
	}
	return &Logs{Activity: file, System: system, Agents: agents, AgentsShort: short}, nil
}

// Remember only token hashes and display metadata, including after revocation.
// This distinguishes actual capabilities from equally long task IDs and hashes.
type activityLogger struct {
	logger *log.Logger
	agents *AgentLogger
	mu     sync.RWMutex
	tasks  map[string]Task
}

func newActivityLogger(output io.Writer) *activityLogger {
	return &activityLogger{logger: log.New(output, "", 0), tasks: make(map[string]Task)}
}

func (l *activityLogger) remember(token string, task Task) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tasks[hash(token)] = Task{ID: task.ID, Skill: task.Skill, Title: task.Title}
}

func (l *activityLogger) task(token string) (Task, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	task, ok := l.tasks[hash(token)]
	return task, ok
}

var possibleToken = regexp.MustCompile(`[0-9a-f]{64}`)

func (l *activityLogger) redact(text string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return possibleToken.ReplaceAllStringFunc(text, func(value string) string {
		if _, ok := l.tasks[hash(value)]; ok {
			return "[redacted]"
		}
		return value
	})
}

func (l *activityLogger) printf(task Task, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	l.write(task, message, message)
}

// Detailed payloads stay in the full logs; the handler's summary already
// describes the outcome in the short log.
func (l *activityLogger) detailf(task Task, format string, args ...any) {
	l.write(task, fmt.Sprintf(format, args...), "")
}

func (l *activityLogger) assignment(task, caller Task, prompt string) {
	parent := displaySkill(caller.Skill) + "/" + shortTaskID(caller.ID)
	l.write(task, fmt.Sprintf("Task prompt assigned by %s:\n%s", parent, prompt),
		fmt.Sprintf("Task delegated by %s", parent))
}

func (l *activityLogger) write(task Task, message, shortMessage string) {
	skill := task.Skill
	if skill == "" {
		skill = "anonymous"
	}
	// Shorten skill names only for display, leaving the protocol registry intact.
	message = l.redact(message)
	skill = displaySkill(l.text(skill))
	at := time.Now()
	prefix := fmt.Sprintf("%s [%s task=%s] ", at.Local().Format("2006/01/02 15:04:05"), skill, shortTaskID(task.ID))
	l.logger.Print(formatReadableRecord(prefix, message))
	if l.agents != nil {
		task.Skill = skill
		l.agents.mcp(at, task, message, l.redact(shortMessage))
	}
}

// Log the handler's actual response once, without the duplicate MCP text and
// structuredContent envelopes. YAML keeps multiline source and nested reports
// readable. Only the display copy is decoded/redacted; callers receive the
// original values, including capabilities and JSON-encoded strings.
func (l *activityLogger) response(task Task, tool string, result any, callErr error) {
	if callErr != nil {
		result = map[string]any{"error": callErr.Error()}
	}
	data, err := json.Marshal(l.payload(result))
	var document yaml.Node
	if err == nil {
		err = yaml.Unmarshal(data, &document)
	}
	if err != nil {
		l.detailf(task, "%s response: [payload unavailable]", tool)
		return
	}
	expandResponseJSON(&document)
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		l.detailf(task, "%s response: [payload unavailable]", tool)
		return
	}
	l.detailf(task, "%s response:\n%s", tool, output.String())
}

// State files and worker reports are JSON strings in the protocol. Expand these
// fields for display so a plan read exposes findings instead of escaped JSON.
func expandResponseJSON(node *yaml.Node) {
	// JSON is parsed as YAML flow nodes. Use block style for readable fields,
	// arrays and literal multiline strings in the log.
	node.Style = 0
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i].Value, node.Content[i+1]
			if (key == "result" || key == "content") && value.Tag == "!!str" && json.Valid([]byte(value.Value)) {
				var decoded yaml.Node
				if yaml.Unmarshal([]byte(value.Value), &decoded) == nil && len(decoded.Content) == 1 {
					*value = *decoded.Content[0]
				}
			}
		}
	}
	for _, child := range node.Content {
		expandResponseJSON(child)
	}
}

func shortTaskID(id string) string {
	if id == "" {
		return "-"
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func (l *activityLogger) target(task Task) string {
	return fmt.Sprintf("%q (%s task=%s)", l.text(task.Title), displaySkill(l.text(task.Skill)), shortTaskID(task.ID))
}

func displaySkill(skill string) string {
	return strings.ReplaceAll(skill, "edict-next-", "edict-")
}

// Keep free-form titles, results and errors on one readable line. Capabilities
// must stay private even if a caller accidentally includes one in prose.
func (l *activityLogger) text(text string) string {
	text = strings.Join(strings.Fields(l.redact(text)), " ")
	runes := []rune(text)
	if len(runes) > 500 {
		return string(runes[:500]) + "…"
	}
	return text
}

// Keep a short status summary above the complete response body.
func (l *activityLogger) result(result string) string {
	var value any
	if json.Unmarshal([]byte(result), &value) != nil {
		return l.text(result)
	}
	switch v := value.(type) {
	case string:
		return l.text(v)
	case map[string]any:
		for _, key := range []string{"summary", "message", "description", "reason"} {
			if summary, ok := v[key].(string); ok && strings.TrimSpace(summary) != "" {
				return l.text(summary)
			}
		}
	}
	return "Structured result recorded"
}

func requestLogging(logger *slog.Logger, activity *activityLogger) mcp.Middleware {
	var sequence atomic.Uint64
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			start := time.Now()
			log := logger.With("requestId", sequence.Add(1), "sessionId", request.GetSession().ID(), "method", method)
			notification := strings.HasPrefix(method, "notifications/")
			message := "request"
			if notification {
				message = "notification"
			}
			log.InfoContext(ctx, message, "params", activity.payload(request.GetParams()))
			result, err := next(ctx, method, request)
			if !notification || err != nil {
				fields := []any{"result", activity.payload(result), "durationMs", time.Since(start).Milliseconds()}
				if err != nil {
					fields = append(fields, "error", activity.redact(err.Error()))
				}
				log.InfoContext(ctx, "response", fields...)
			}
			return result, err
		}
	}
}

// Redact copies of payloads, including JSON encoded inside MCP text content.
// The actual request and response must retain the capabilities for the caller.
func (l *activityLogger) payload(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return "[payload unavailable]"
	}
	var copy any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&copy); err != nil {
		return "[payload unavailable]"
	}
	return l.redactTokens(copy)
}

func (l *activityLogger) redactTokens(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, field := range v {
			switch key {
			case "token", "managerToken", "manager_token":
				v[key] = "[redacted]"
			default:
				v[key] = l.redactTokens(field)
			}
		}
	case []any:
		for i, field := range v {
			v[i] = l.redactTokens(field)
		}
	case string:
		// Tool text may duplicate structuredContent, including its token fields.
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			if json.Valid([]byte(trimmed)) {
				data, err := json.Marshal(l.payload(json.RawMessage(trimmed)))
				if err == nil {
					return string(data)
				}
			}
		}
		return l.redact(v)
	}
	return value
}
