// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentLoggerAttributesFullOutputAndRedactsRevokedTokens(t *testing.T) {
	store, manager, _ := testStore(t)
	var output, short bytes.Buffer
	logger := NewAgentLogger(store, &output, &short)
	now := time.Now()
	logger.Record(AgentMessage{Time: now, AgentID: "manager-thread", IsRoot: true, Phase: "commentary", Text: "Starting managed analysis\n" + strings.Repeat("Multiline commentary 世界. ", 80)})
	logger.Record(AgentMessage{Time: now, AgentID: "worker-thread", AgentPath: "/root/batch", Phase: "commentary", Text: "Inspecting commit"})
	require.NoError(t, logger.Flush(false))
	require.Contains(t, output.String(), "[edict_manager/-] commentary: Starting managed analysis")
	require.NotContains(t, output.String(), "Inspecting commit")

	grant, err := delegateTestTask(store, manager, store.Plan().Tasks[0].ID, nil, nil)
	require.NoError(t, err)
	_, err = store.StartTask(grant.Token, "/root/batch", grant.Skill) // Some runtimes return paths, others UUIDs.
	require.NoError(t, err)
	require.NoError(t, logger.Flush(false))
	prefix := "[edict-batch-signal-analysis/" + grant.TaskID[:8] + "] "
	require.Contains(t, output.String(), prefix+"commentary: Inspecting commit")

	_, err = store.FinishTask(grant.Token, "completed", "Done")
	require.NoError(t, err)
	hash := strings.Repeat("f", 64)
	long := strings.Repeat("Detailed output. ", 80)
	logger.Record(AgentMessage{Time: now, AgentID: "worker-thread", AgentPath: "/root/batch", Phase: "final_answer",
		Text: long + "\nPlan " + store.Plan().ID + "; hash " + hash + "; tokens " + manager + " " + grant.Token})
	logger.Record(AgentMessage{Time: now, AgentID: "lost-worker", Text: "Could not start"})
	require.NoError(t, logger.Flush(true))
	unwrapped := strings.ReplaceAll(output.String(), "\n    ", "")
	require.Contains(t, unwrapped, prefix+"final: "+long)
	require.Contains(t, unwrapped, "Plan "+store.Plan().ID+"; hash "+hash+"; tokens [redacted] [redacted]")
	require.Contains(t, output.String(), "[unassigned/-] message: Could not start")
	require.NotContains(t, output.String(), manager)
	require.NotContains(t, output.String(), grant.Token)
	require.NotContains(t, output.String(), "agent=")
	require.Equal(t, 1, strings.Count(output.String(), "Inspecting commit"))
	previous := output.String()
	require.NoError(t, logger.Flush(true))
	require.Equal(t, previous, output.String(), "flushing must not duplicate messages")
	require.Equal(t, output.String(), short.String(), "commentary and final messages must be identical in both logs")
}

type failingAgentLogWriter struct{}

func (failingAgentLogWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestAgentLoggerReportsWriteFailure(t *testing.T) {
	store, _, _ := testStore(t)
	for _, shortFails := range []bool{false, true} {
		var output bytes.Buffer
		var full, short io.Writer = failingAgentLogWriter{}, &output
		if shortFails {
			full, short = short, full
		}
		logger := NewAgentLogger(store, full, short)
		logger.Record(AgentMessage{Time: time.Now(), AgentID: "root", IsRoot: true, Text: "Output"})
		require.ErrorContains(t, logger.Flush(true), "disk full")
		previous := output.String()
		require.ErrorContains(t, logger.Flush(true), "disk full")
		require.Equal(t, previous, output.String(), "retrying after a write failure must not duplicate messages")
	}
}
