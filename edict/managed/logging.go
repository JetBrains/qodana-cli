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
)

// Logs keeps readable tool activity separate from detailed protocol diagnostics.
type Logs struct {
	Activity *os.File
	System   *os.File
}

func (l *Logs) Close() error {
	return errors.Join(l.Activity.Close(), l.System.Close())
}

// OpenLogs appends both logs below the caller's log directory. The caller must
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
	return &Logs{Activity: file, System: system}, nil
}

// Remember only token hashes and display metadata, including after revocation.
// This distinguishes actual capabilities from equally long task IDs and hashes.
type activityLogger struct {
	logger *log.Logger
	mu     sync.RWMutex
	tasks  map[string]Task
}

func newActivityLogger(output io.Writer) *activityLogger {
	return &activityLogger{logger: log.New(output, "", log.LstdFlags), tasks: make(map[string]Task)}
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
	skill := task.Skill
	if skill == "" {
		skill = "anonymous"
	}
	// Shorten skill names only for display, leaving the protocol registry intact.
	message := strings.ReplaceAll(l.redact(fmt.Sprintf(format, args...)), "edict-next-", "edict-")
	skill = strings.ReplaceAll(l.text(skill), "edict-next-", "edict-")
	l.logger.Printf("[%s task=%s] %s", skill, shortTaskID(task.ID), message)
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
	return fmt.Sprintf("%q (%s task=%s)", l.text(task.Title), l.text(task.Skill), shortTaskID(task.ID))
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

// Workers may return a JSON report as their result string. Keep its technical
// details in the system log and show only a prose summary in the activity log.
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
