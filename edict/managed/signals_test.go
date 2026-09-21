// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func validTestSignal(t *testing.T, key string) File {
	t.Helper()
	var signal signalRecord
	signal.ID = "s-" + hash(key)[:10]
	signal.IdempotencyKey, signal.Label, signal.Description = key, "POSITIVE", "Wait for completion"
	signal.FileRevision.Path = "project/src/Waiter.java"
	signal.FileRevision.Revision = strings.Repeat("a", 40)
	signal.FileRevision.ExpectedRanges = []signalRange{{Start: 1, End: 1}}
	signal.Source.Type = "FromCommit"
	signal.Source.ParentRevision = signal.FileRevision.Revision
	signal.Source.CommitRevision = strings.Repeat("b", 40)
	signal.Source.Message = "Replace sleeping with synchronization"
	signal.Source.DiffPositiveToNegative = "diff --git a/project/src/Waiter.java b/project/src/Waiter.java\n--- a/project/src/Waiter.java\n+++ b/project/src/Waiter.java\n@@ -1,2 +1,2 @@\n-sleep();\n+await();\n end();\n"
	signal.Provenance.WorkItemID = "commit-bbbbbbbbbbbbbbbb"
	data, err := json.Marshal(signal)
	require.NoError(t, err)
	return File{Path: "inbox/" + signal.ID + ".json", Content: string(data)}
}

func changeTestSignal(t *testing.T, content string, edit func(*signalRecord)) string {
	t.Helper()
	var signal signalRecord
	require.NoError(t, json.Unmarshal([]byte(content), &signal))
	edit(&signal)
	data, err := json.Marshal(signal)
	require.NoError(t, err)
	return string(data)
}

func TestSignalWriteValidation(t *testing.T) {
	s, manager, _ := testStore(t)
	batch := worker(t, s, manager, "edict-next-batch-signal-analysis", []string{"inbox.write"}, []string{"inbox"})
	for _, test := range []struct {
		name, field string
		edit        func(*signalRecord)
	}{
		{"project-relative path", "fileRevision.path", func(s *signalRecord) { s.FileRevision.Path = "src/Waiter.java" }},
		{"absolute path", "fileRevision.path", func(s *signalRecord) { s.FileRevision.Path = "/project/src/Waiter.java" }},
		{"empty description", "description", func(s *signalRecord) { s.Description = " " }},
		{"empty key", "idempotencyKey", func(s *signalRecord) { s.IdempotencyKey = "" }},
		{"wrong ID", "id", func(s *signalRecord) { s.ID = "s-wrong" }},
		{"missing work item", "provenance.workItemId", func(s *signalRecord) { s.Provenance.WorkItemID = "" }},
		{"unsupported label", "label", func(s *signalRecord) { s.Label = "OTHER" }},
		{"abbreviated revision", "fileRevision.revision", func(s *signalRecord) { s.FileRevision.Revision = "abcdef" }},
		{"wrong evidence side", "fileRevision.revision", func(s *signalRecord) { s.FileRevision.Revision = s.Source.CommitRevision }},
		{"missing parent", "source.parentRevision", func(s *signalRecord) { s.Source.ParentRevision = "" }},
		{"missing message", "source.message", func(s *signalRecord) { s.Source.Message = "" }},
		{"missing ranges", "fileRevision.expectedRanges", func(s *signalRecord) { s.FileRevision.ExpectedRanges = nil }},
		{"zero range", "fileRevision.expectedRanges[0]", func(s *signalRecord) { s.FileRevision.ExpectedRanges[0].Start = 0 }},
		{"reversed range", "fileRevision.expectedRanges[0]", func(s *signalRecord) { s.FileRevision.ExpectedRanges[0].End = 0 }},
		{"unchanged range", "fileRevision.expectedRanges[0]", func(s *signalRecord) { s.FileRevision.ExpectedRanges = []signalRange{{2, 2}} }},
		{"missing diff", "source.diffPositiveToNegative", func(s *signalRecord) { s.Source.DiffPositiveToNegative = "" }},
		{"truncated diff", "source.diffPositiveToNegative", func(s *signalRecord) {
			s.Source.DiffPositiveToNegative = strings.TrimSuffix(s.Source.DiffPositiveToNegative, " end();\n")
		}},
		{"unknown source", "source.type", func(s *signalRecord) { s.Source.Type = "unknown" }},
		{"incomplete PR", "source", func(s *signalRecord) { s.Source.Type = "FromPR" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := validTestSignal(t, test.name)
			invalid := changeTestSignal(t, file.Content, test.edit)
			_, err := s.Write(batch.Token, file.Path, invalid, "")
			require.ErrorContains(t, err, "invalid signal")
			require.ErrorContains(t, err, test.field+":")
			_, err = s.Read(file.Path)
			require.Error(t, err, "invalid creation must not leave an artifact")
			stored, err := s.Write(batch.Token, file.Path, file.Content, "")
			require.NoError(t, err, "valid retry must succeed")
			_, err = s.Write(batch.Token, file.Path, invalid, stored.Hash)
			require.ErrorContains(t, err, test.field+":")
			unchanged, err := s.Read(file.Path)
			require.NoError(t, err)
			require.Equal(t, stored, unchanged, "invalid replacement must preserve both content and hash")
		})
	}
}

func TestSignalValidationAtMCPWrite(t *testing.T) {
	session, store, ctx := connectTestServer(t, nil)
	created, err := store.CreatePlan("Extract signals", []Step{{Skill: "edict-next-batch-signal-analysis", Title: "Extract signals"}})
	require.NoError(t, err)
	batch := worker(t, store, created.Token, "edict-next-batch-signal-analysis", []string{"inbox.write"}, []string{"inbox"})
	file := validTestSignal(t, "path-regression")
	invalid := changeTestSignal(t, file.Content, func(s *signalRecord) { s.FileRevision.Path = "src/Waiter.java" })
	args := map[string]any{"token": batch.Token, "path": file.Path, "content": invalid, "expectedHash": ""}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "edict_state_write", Arguments: args})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, `fileRevision.path: got "src/Waiter.java"`)
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, `project/src/Waiter.java`)
	_, err = store.Read(file.Path)
	require.Error(t, err)
	args["content"] = file.Content
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "edict_state_write", Arguments: args})
	require.NoError(t, err)
	require.False(t, result.IsError, "%v", result.GetError())
	stored, err := store.Read(file.Path)
	require.NoError(t, err)
	require.Equal(t, file.Content, stored.Content)
}

func TestSignalDiffEvidence(t *testing.T) {
	for _, test := range []struct {
		name, before, after, hunk, label, wantPath string
		wantLine                                   int
	}{
		{"rename before", "a/old.java", "b/new.java", "@@ -1 +1 @@\n-old\n+new\n", "POSITIVE", "old.java", 1},
		{"rename after", "a/old.java", "b/new.java", "@@ -1 +1 @@\n-old\n+new\n", "NEGATIVE", "new.java", 1},
		{"addition", "/dev/null", "b/new.java", "@@ -0,0 +1 @@\n+new\n", "NEGATIVE", "new.java", 1},
		{"deletion", "a/old.java", "/dev/null", "@@ -1 +0,0 @@\n-old\n", "POSITIVE", "old.java", 1},
		{"quoted path", `"a/sp ace/\303\251.java"`, `"b/sp ace/\303\251.java"`, "@@ -1 +1 @@\n--- old\n+++ new\n", "POSITIVE", "sp ace/é.java", 1},
		{"no newline", "a/a.java", "b/a.java", "@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n", "NEGATIVE", "a.java", 1},
		{"multiple hunks", "a/a.java", "b/a.java", "@@ -1 +1 @@\n-old\n+new\n@@ -8 +8 @@\n-old\n+new\n", "NEGATIVE", "a.java", 8},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := validTestSignal(t, test.name)
			content := changeTestSignal(t, file.Content, func(s *signalRecord) {
				s.Label, s.FileRevision.Path = test.label, test.wantPath
				if test.label == "NEGATIVE" {
					s.FileRevision.Revision = s.Source.CommitRevision
				}
				s.FileRevision.ExpectedRanges = []signalRange{{test.wantLine, test.wantLine}}
				s.Source.DiffPositiveToNegative = fmt.Sprintf("--- %s\n+++ %s\n%s", test.before, test.after, test.hunk)
			})
			require.NoError(t, validateSignal(file.Path, content))
		})
	}
	file := validTestSignal(t, "PR")
	content := changeTestSignal(t, file.Content, func(s *signalRecord) {
		s.Source.Type, s.Source.PRNumber, s.Source.Title, s.Source.URL = "FromPR", 1, "Fix wait", "https://example.test/pull/1"
		s.Source.DiscussionMessages = []json.RawMessage{json.RawMessage(`"Use explicit synchronization"`)}
	})
	require.NoError(t, validateSignal(file.Path, content))
}
