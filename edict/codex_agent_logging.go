// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package edict

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JetBrains/qodana-cli/edict/managed"
)

func startCodexAgentLogging(config CodexRunConfig, stdoutPath string) func() error {
	if config.AgentLogger == nil {
		return func() error { return nil }
	}
	collector := newCodexAgentCollector(config.HomeDirectory, stdoutPath, config.AgentLogger)
	stop, done := make(chan struct{}), make(chan error, 1)
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := collector.scan(false); err != nil {
					done <- err
					return
				}
			case <-stop:
				done <- collector.scan(true)
				return
			}
		}
	}()
	return func() error {
		close(stop)
		return <-done
	}
}

type codexAgentCollector struct {
	home, rootID string
	stdout       *codexJSONLTail
	logger       *managed.AgentLogger
	files        map[string]*codexAgentSession
}

type codexAgentSession struct {
	tail             *codexJSONLTail
	id, parent, path string
	messages         []managed.AgentMessage
}

func newCodexAgentCollector(home, stdout string, logger *managed.AgentLogger) *codexAgentCollector {
	return &codexAgentCollector{home: home, stdout: &codexJSONLTail{path: stdout}, logger: logger, files: make(map[string]*codexAgentSession)}
}

func (c *codexAgentCollector) scan(final bool) error {
	if err := c.stdout.read(final, func(line []byte) error {
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			return err
		}
		if event.Type == "thread.started" {
			c.rootID = event.ThreadID
		}
		return nil
	}); err != nil {
		return err
	}
	err := filepath.WalkDir(filepath.Join(c.home, "sessions"), func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, fs.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		session := c.files[path]
		if session == nil {
			session = &codexAgentSession{tail: &codexJSONLTail{path: path}}
			c.files[path] = session
		}
		return session.tail.read(final, session.consume)
	})
	if err != nil {
		return fmt.Errorf("read Codex agent output: %w", err)
	}
	// Use runtime ancestry, never cwd or the most recently active worker, to
	// exclude unrelated sessions in the same CODEX_HOME.
	allowed := map[string]bool{c.rootID: c.rootID != ""}
	for changed := true; changed; {
		changed = false
		for _, session := range c.files {
			if session.id != "" && allowed[session.parent] && !allowed[session.id] {
				allowed[session.id], changed = true, true
			}
		}
	}
	for _, session := range c.files {
		if !allowed[session.id] {
			continue
		}
		for _, message := range session.messages {
			message.AgentID, message.AgentPath, message.IsRoot = session.id, session.path, session.id == c.rootID
			c.logger.Record(message)
		}
		session.messages = nil
	}
	return c.logger.Flush(final)
}

func (s *codexAgentSession) consume(line []byte) error {
	var event struct {
		Time    time.Time       `json:"timestamp"`
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return err
	}
	if event.Type == "session_meta" {
		var meta struct {
			ID     string          `json:"id"`
			Source json.RawMessage `json:"source"`
		}
		if err := json.Unmarshal(event.Payload, &meta); err != nil {
			return err
		}
		s.id = meta.ID
		var source struct {
			Subagent struct {
				Spawn struct {
					Parent string `json:"parent_thread_id"`
					Path   string `json:"agent_path"`
				} `json:"thread_spawn"`
			} `json:"subagent"`
		}
		// Root session sources are strings such as "exec".
		if len(meta.Source) > 0 && meta.Source[0] == '{' {
			if err := json.Unmarshal(meta.Source, &source); err != nil {
				return err
			}
			s.parent, s.path = source.Subagent.Spawn.Parent, source.Subagent.Spawn.Path
		}
		return nil
	}
	if event.Type != "response_item" {
		return nil
	}
	var message struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Phase   string          `json:"phase"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(event.Payload, &message); err != nil {
		return err
	}
	if message.Type != "message" || message.Role != "assistant" ||
		(message.Phase != "" && message.Phase != "commentary" && message.Phase != "final" && message.Phase != "final_answer") {
		return nil
	}
	var content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(message.Content, &content); err != nil {
		return err
	}
	var text []string
	for _, part := range content {
		if part.Type == "output_text" {
			text = append(text, part.Text)
		}
	}
	if joined := strings.Join(text, "\n"); strings.TrimSpace(joined) != "" {
		s.messages = append(s.messages, managed.AgentMessage{Time: event.Time, Phase: message.Phase, Text: joined})
	}
	return nil
}

// Keep incomplete JSONL records until their newline arrives; a live writer may
// split one event across multiple writes. Each complete record is consumed once.
type codexJSONLTail struct {
	path    string
	offset  int64
	pending []byte
}

func (t *codexJSONLTail) read(final bool, consume func([]byte) error) error {
	file, err := os.Open(t.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Seek(t.offset, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	t.offset += int64(len(data))
	t.pending = append(t.pending, data...)
	for {
		end := bytes.IndexByte(t.pending, '\n')
		if end < 0 {
			break
		}
		if line := bytes.TrimSpace(t.pending[:end]); len(line) > 0 {
			if err := consume(line); err != nil {
				return fmt.Errorf("parse %s: %w", t.path, err)
			}
		}
		t.pending = t.pending[end+1:]
	}
	if final && len(bytes.TrimSpace(t.pending)) > 0 {
		if err := consume(t.pending); err != nil {
			return fmt.Errorf("parse final record in %s: %w", t.path, err)
		}
		t.pending = nil
	}
	return nil
}
