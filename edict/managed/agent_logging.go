// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"fmt"
	"io"
	"maps"
	"sort"
	"sync"
	"time"
)

// AgentMessage is an emitted assistant message supplied by the trusted runtime.
// It contains commentary/final output, never prompts, tool payloads, or reasoning.
type AgentMessage struct {
	Time      time.Time
	AgentID   string
	AgentPath string
	IsRoot    bool
	Phase     string
	Text      string
}

// AgentLogger correlates runtime output with managed tasks and redacts issued
// capabilities, including revoked ones. The caller owns the output writers.
type AgentLogger struct {
	mu          sync.Mutex
	store       *Store
	output      io.Writer
	shortOutput io.Writer
	identities  map[string]Task
	pending     []AgentMessage
	err         error
}

// NewAgentLogger writes full records to output and MCP summaries with unchanged
// agent messages to shortOutput. A nil writer discards that log.
func NewAgentLogger(store *Store, output, shortOutput io.Writer) *AgentLogger {
	if output == nil {
		output = io.Discard
	}
	if shortOutput == nil {
		shortOutput = io.Discard
	}
	return &AgentLogger{store: store, output: output, shortOutput: shortOutput, identities: make(map[string]Task)}
}

func (l *AgentLogger) Record(message AgentMessage) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending = append(l.pending, message)
}

// Flush defers early worker messages until task_start supplies their identity.
// At runtime shutdown, final=true preserves unassigned messages rather than
// dropping output from workers that failed before starting.
func (l *AgentLogger) Flush(final bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return l.err
	}
	l.store.mu.Lock()
	if l.store.plan != nil {
		for _, task := range l.store.plan.Tasks {
			if task.AgentID != "" {
				l.identities[task.AgentID] = Task{ID: task.ID, Skill: task.Skill}
			}
		}
	}
	secrets := maps.Clone(l.store.issuedTokens)
	l.store.mu.Unlock()
	sort.SliceStable(l.pending, func(i, j int) bool { return l.pending[i].Time.Before(l.pending[j].Time) })
	var pending []AgentMessage
	for _, message := range l.pending {
		task, known := l.identities[message.AgentID]
		if !known {
			task, known = l.identities[message.AgentPath]
		}
		if message.IsRoot {
			task, known = Task{Skill: "edict_manager"}, true
		}
		if !known && !final {
			pending = append(pending, message)
			continue
		}
		if !known {
			task.Skill = "unassigned"
		}
		text := possibleToken.ReplaceAllStringFunc(message.Text, func(value string) string {
			if _, secret := secrets[hash(value)]; secret {
				return "[redacted]"
			}
			return value
		})
		phase := message.Phase
		if phase == "final_answer" {
			phase = "final"
		}
		if phase == "" {
			phase = "message"
		}
		if l.err = l.write(message.Time, task, phase, text, text); l.err != nil {
			return l.err
		}
	}
	l.pending = pending
	return nil
}

// MCP records are already attributed and redacted by the tool handler. Share
// the same lock and formatter as runtime output so records cannot interleave.
func (l *AgentLogger) mcp(at time.Time, task Task, text, summary string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err == nil {
		l.err = l.write(at, task, "mcp", text, summary)
	}
}

func (l *AgentLogger) write(at time.Time, task Task, kind, text, shortText string) error {
	record := formatAgentRecord(at, task, kind, text)
	if _, err := io.WriteString(l.output, record); err != nil {
		return fmt.Errorf("write managed agent output: %w", err)
	}
	if shortText != "" || kind != "mcp" {
		if shortText != text {
			record = formatAgentRecord(at, task, kind, shortText)
		}
		if _, err := io.WriteString(l.shortOutput, record); err != nil {
			return fmt.Errorf("write managed short agent output: %w", err)
		}
	}
	return nil
}
