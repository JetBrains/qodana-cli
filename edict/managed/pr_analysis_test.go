// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JetBrains/qodana-cli/edict/managed/review"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func managedPRProvider(t *testing.T, s *Store) *atomic.Int64 {
	t.Helper()
	calls := new(atomic.Int64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "Bearer provider-secret", r.Header.Get("Authorization"))
		var value any
		switch r.URL.Path {
		case "/repos/owner/repo/pulls/7":
			value = map[string]any{"number": 7, "title": "Wait for completion", "body": "Full pull request context", "html_url": "https://github.test/owner/repo/pull/7", "merged_at": "2026-09-21T10:00:00Z", "base": map[string]string{"sha": strings.Repeat("a", 40)}, "head": map[string]string{"sha": strings.Repeat("b", 40)}}
		case "/repos/owner/repo/pulls/7/comments":
			value = []any{
				map[string]any{"id": 10, "path": "project/src/Waiter.java", "original_commit_id": strings.Repeat("a", 40), "original_line": 1, "body": "Use await instead of sleep", "created_at": "2026-09-20T10:00:00Z", "user": map[string]string{"login": "reviewer", "type": "User"}},
				map[string]any{"id": 11, "path": "project/src/Other.java", "original_commit_id": strings.Repeat("a", 40), "original_line": 2, "body": "Thanks", "created_at": "2026-09-20T11:00:00Z", "user": map[string]string{"login": "reviewer", "type": "User"}},
			}
		case "/repos/owner/repo/pulls/7/reviews":
			value = []any{}
		default:
			t.Errorf("unexpected provider request %s", r.URL)
			http.NotFound(w, r)
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(value))
	}))
	t.Cleanup(server.Close)
	s.reviewClient = &review.Client{HTTP: server.Client(), GitHubURL: server.URL, GitHubToken: "provider-secret"}
	return calls
}

func prCall[T any](t *testing.T, session *mcp.ClientSession, ctx context.Context, name string, args any) T {
	t.Helper()
	response, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.False(t, response.IsError, "%s: %+v", name, response.Content)
	return protocolOutput[T](t, response)
}

func TestPRExtractionThroughMCPValidatesCoverageBeforePublication(t *testing.T) {
	logs, err := OpenLogs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, logs.Close()) })
	session, s, ctx := connectTestServer(t, logs)
	providerCalls := managedPRProvider(t, s)
	created := prCall[PlanCreation](t, session, ctx, "edict_plan_create", map[string]any{"request": "Extract signals from GitHub PR 7", "steps": []Step{{Skill: prSkill, Title: "Extract review"}}})
	d := prCall[Delegation](t, session, ctx, "edict_delegate", map[string]any{"token": created.Token, "taskId": created.Plan.Tasks[0].ID, "prompt": testPrompt(prSkill), "operations": []string{"inbox.write"}, "scope": []string{"inbox"}})
	prCall[TaskAssignment](t, session, ctx, "edict_task_get", map[string]any{"token": d.Token})
	prCall[Plan](t, session, ctx, "edict_task_start", map[string]any{"token": d.Token, "agentId": "review-coordinator", "skill": d.Skill})
	batch := prCall[prBatchSummary](t, session, ctx, "edict_prepare_pr_analysis", map[string]any{"token": d.Token, "provider": "github", "owner": "owner", "repo": "repo", "prNumbers": []int{7}, "maxPrs": 1})
	require.Equal(t, 2, batch.TotalWorkItemCount)
	page := prCall[prPage](t, session, ctx, "edict_list_pr_analysis_items", map[string]any{"token": d.Token, "batchId": batch.BatchID, "offset": 0, "limit": 1})
	require.Len(t, page.Items, 1)
	require.NotNil(t, page.NextOffset)
	last := prCall[prPage](t, session, ctx, "edict_list_pr_analysis_items", map[string]any{"token": d.Token, "batchId": batch.BatchID, "offset": *page.NextOffset, "limit": 1})
	require.Nil(t, last.NextOffset)
	ids := []string{page.Items[0].WorkItemID, last.Items[0].WorkItemID}
	reader := worker(t, s, d.Token, "edict-next-signal-analysis", nil, nil)
	item := prCall[prItem](t, session, ctx, "edict_get_pr_analysis_item", map[string]any{"token": reader.Token, "batchId": batch.BatchID, "workItemId": ids[0]})
	require.Equal(t, "Full pull request context", item.PR.Body)
	require.Equal(t, "Use await instead of sleep", item.Thread.Messages[0].Body)
	_, err = s.FinishTask(reader.Token, "completed", "Inspected assigned discussion")
	require.NoError(t, err)
	file := validTestSignal(t, "review-7-positive")
	file.Content = changeTestSignal(t, file.Content, func(signal *signalRecord) {
		signal.Source.Type = "FromPR"
		signal.Source.PRNumber = 7
		signal.Source.Title = item.PR.Title
		signal.Source.URL = item.Thread.URL
		signal.Source.DiscussionMessages = []json.RawMessage{json.RawMessage(`"Use await instead of sleep"`)}
		signal.Provenance.WorkItemID = item.WorkItemID
	})
	write := map[string]any{"token": d.Token, "path": file.Path, "content": file.Content, "expectedHash": ""}
	response, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "edict_state_write", Arguments: write})
	require.NoError(t, err)
	require.True(t, response.IsError, "unvalidated PR signal must not be persisted")
	_, err = s.Read(file.Path)
	require.Error(t, err)
	for _, bad := range [][]string{nil, {ids[0]}, {ids[0], ids[0]}, {ids[1], ids[0]}, {ids[0], "invented"}} {
		response, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "edict_validate_pr_signals", Arguments: map[string]any{"token": d.Token, "batchId": batch.BatchID, "inspectedWorkItemIds": bad, "signals": []string{file.Content}}})
		require.NoError(t, err)
		require.True(t, response.IsError, "incomplete or duplicate coverage must fail: %v", bad)
	}
	for _, edit := range []func(*signalRecord){
		func(s *signalRecord) { s.Source.Title = "invented" }, func(s *signalRecord) { s.Source.DiscussionMessages = []json.RawMessage{json.RawMessage(`"summary"`)} },
		func(s *signalRecord) { s.FileRevision.Revision = strings.Repeat("b", 40) }, func(s *signalRecord) { s.Provenance.WorkItemID = "invented" },
	} {
		_, err := s.validatePR(d.Token, batch.BatchID, ids, []string{changeTestSignal(t, file.Content, edit)})
		require.Error(t, err)
	}
	receipt := prCall[prReceipt](t, session, ctx, "edict_validate_pr_signals", map[string]any{"token": d.Token, "batchId": batch.BatchID, "inspectedWorkItemIds": ids, "signals": []string{file.Content}})
	require.Equal(t, hash(file.Content), receipt.Signals[file.Path])
	_, err = s.FinishTask(d.Token, "completed", "Done")
	require.ErrorContains(t, err, "missing")
	write["content"] = file.Content + "\n"
	response, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "edict_state_write", Arguments: write})
	require.NoError(t, err)
	require.True(t, response.IsError, "changed bytes require validation again")
	write["content"] = file.Content
	written := prCall[File](t, session, ctx, "edict_state_write", write)
	require.Equal(t, receipt.Signals[file.Path], written.Hash)
	prCall[Plan](t, session, ctx, "edict_task_finish", map[string]any{"token": d.Token, "status": "completed", "result": "One signal from two inspected discussions"})
	require.Equal(t, int64(3), providerCalls.Load(), "paging and validation must use the retained provider snapshot")
	_, err = s.getPR(d.Token, batch.BatchID, ids[0])
	require.ErrorContains(t, err, "revoked")
	for _, log := range []*os.File{logs.Activity, logs.System, logs.Agents, logs.AgentsShort} {
		content, err := os.ReadFile(log.Name())
		require.NoError(t, err)
		assert.NotContains(t, string(content), "provider-secret")
		assert.NotContains(t, string(content), d.Token)
		assert.NotContains(t, string(content), reader.Token)
	}
	full, err := os.ReadFile(logs.Agents.Name())
	require.NoError(t, err)
	assert.Contains(t, string(full), "edict_get_pr_analysis_item response:")
	assert.Contains(t, string(full), "Full pull request context")
	short, err := os.ReadFile(logs.AgentsShort.Name())
	require.NoError(t, err)
	assert.Contains(t, string(short), "Validated PR coverage: 2 discussions, 1 signals")
	assert.NotContains(t, string(short), "Full pull request context")
}

func TestPRBatchAccessIsLimitedToItsTaskAndChildren(t *testing.T) {
	s, manager, _ := testStore(t)
	calls := managedPRProvider(t, s)
	commit := worker(t, s, manager, "edict-next-batch-signal-analysis", []string{"inbox.write"}, []string{"inbox"})
	selection := review.Selection{Repository: review.Repository{Provider: "github", Owner: "owner", Repo: "repo"}, PRNumbers: []int{7}, MaxPRs: 1}
	_, err := s.preparePR(context.Background(), commit.Token, selection)
	require.ErrorContains(t, err, prSkill)
	require.Zero(t, calls.Load())
	coordinator := worker(t, s, manager, prSkill, []string{"inbox.write"}, []string{"inbox"})
	_, err = s.FinishTask(coordinator.Token, "completed", "Nothing to do")
	require.ErrorContains(t, err, "prepare")
	batch, err := s.preparePR(context.Background(), coordinator.Token, selection)
	require.NoError(t, err)
	other := worker(t, s, manager, prSkill, []string{"inbox.write"}, []string{"inbox"})
	_, err = s.listPR(other.Token, batch.BatchID, 0, 20)
	require.ErrorContains(t, err, "unknown PR batch")
	reader := worker(t, s, coordinator.Token, "edict-next-signal-analysis", nil, nil)
	page, err := s.listPR(reader.Token, batch.BatchID, 0, 20)
	require.NoError(t, err)
	_, err = s.validatePR(reader.Token, batch.BatchID, nil, nil)
	require.Error(t, err, "read-only evidence worker cannot finalize a batch")
	_, err = s.prSourceItem(reader.Token, batch.BatchID, page.Items[0].WorkItemID, strings.Repeat("c", 40))
	require.ErrorContains(t, err, "revision does not belong")
	file := validTestSignal(t, "commit-cannot-write-pr")
	file.Content = changeTestSignal(t, file.Content, func(signal *signalRecord) {
		signal.Source.Type = "FromPR"
		signal.Source.PRNumber = 7
		signal.Source.Title = "title"
		signal.Source.URL = "https://github.test/pr/7"
		signal.Source.DiscussionMessages = []json.RawMessage{json.RawMessage(`"comment"`)}
	})
	_, err = s.Write(commit.Token, file.Path, file.Content, "")
	require.ErrorContains(t, err, "PR signals must be extracted through")
}

func TestPRZeroFindingsAndStableBatchIdentity(t *testing.T) {
	s, manager, _ := testStore(t)
	managedPRProvider(t, s)
	d := worker(t, s, manager, prSkill, []string{"inbox.write"}, []string{"inbox"})
	selection := review.Selection{Repository: review.Repository{Provider: "github", Owner: "owner", Repo: "repo"}, PRNumbers: []int{7}, MaxPRs: 1}
	batch, err := s.preparePR(context.Background(), d.Token, selection)
	require.NoError(t, err)
	again, err := s.preparePR(context.Background(), d.Token, selection)
	require.NoError(t, err)
	require.Equal(t, batch, again)
	page, err := s.listPR(d.Token, batch.BatchID, 0, 20)
	require.NoError(t, err)
	ids := []string{page.Items[0].WorkItemID, page.Items[1].WorkItemID}
	_, err = s.FinishTask(d.Token, "completed", "No signals")
	require.ErrorContains(t, err, "coverage")
	receipt, err := s.validatePR(d.Token, batch.BatchID, ids, []string{})
	require.NoError(t, err)
	require.Zero(t, receipt.SignalCount)
	_, err = s.FinishTask(d.Token, "completed", "Both discussions inspected; no supported correction")
	require.NoError(t, err)
	paths, err := s.List("inbox")
	require.NoError(t, err)
	require.Empty(t, paths)
}
