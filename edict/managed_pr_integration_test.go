// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package edict

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real Codex and native workers use the real edict-mcp provider clients against
// deterministic HTTP review fixtures. Historical source comes from real Git.
func TestManagedEdictExtractSignalsFromPRReview(t *testing.T) {
	if testing.Short() {
		t.Skip("real Codex execution is long")
	}
	requireCodexProvider(t)
	for _, provider := range []string{"github", "space"} {
		t.Run(provider, func(t *testing.T) {
			fixture := prepareManagedReviewProvider(t, provider)
			project := prepareManagedTestProject(t, historyFixtureFixRevision, historyFixtureProject)
			for _, revision := range []string{historyFixtureSnapshot, historyFixtureFixRevision} {
				fixture.Source[revision] = runCommand(t, project.Checkout.RepositoryDirectory, project.Checkout.GitBinary,
					"show", revision+":"+historyFixtureSourcePath)
			}
			codex := prepareManagedCodex(t, project)
			result := codex.run(t, fmt.Sprintf("Extract signals from %s review #7 in owner/repo.", provider), 15*time.Minute)
			codex.assertCompletedTasks(t, result, "edict-next-pr-signal-analysis", "edict-next-signal-analysis")
			plan := project.Store.Plan()
			require.Len(t, plan.Tasks, 2, "one PR coordinator and one discussion worker")
			assert.Equal(t, "edict-next-pr-signal-analysis", plan.Tasks[0].Skill)
			assert.Equal(t, plan.Tasks[0].ID, plan.Tasks[1].ParentID)
			assertManagedReviewSignals(t, project, fixture)
			assertManagedReviewCalls(t, project.Checkout.TestRoot)
			assertManagedCheckoutUnchanged(t, project)
		})
	}
}

type managedReviewFixture struct {
	Title, Message, URL string
	Source              map[string]string
}

func prepareManagedReviewProvider(t *testing.T, provider string) managedReviewFixture {
	t.Helper()
	fixture := managedReviewFixture{Title: "Replace Thread.sleep with explicit worker synchronization", Message: "Please replace Thread.sleep with an explicit worker completion signal. The final commit uses CountDownLatch.await instead.", Source: make(map[string]string)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer fixture-review-token", r.Header.Get("Authorization"))
		var value any
		switch r.URL.Path {
		case "/repos/owner/repo/pulls/7":
			value = map[string]any{"number": 7, "title": fixture.Title, "body": "Correct worker synchronization", "html_url": "https://github.test/owner/repo/pull/7", "merged_at": "2026-09-21T12:00:00Z", "base": map[string]string{"sha": historyFixtureSnapshot}, "head": map[string]string{"sha": historyFixtureFixRevision}}
		case "/repos/owner/repo/pulls/7/comments":
			value = []any{map[string]any{"id": 10, "path": historyFixtureSourcePath, "original_commit_id": historyFixtureSnapshot, "original_line": 5, "body": fixture.Message, "created_at": "2026-09-20T12:00:00Z", "user": map[string]string{"login": "reviewer", "type": "User"}}}
		case "/repos/owner/repo/pulls/7/reviews":
			value = []any{}
		case "/repos/owner/repo/contents/" + historyFixtureSourcePath:
			content, exists := fixture.Source[r.URL.Query().Get("ref")]
			if !exists {
				http.NotFound(w, r)
				return
			}
			value = map[string]any{"type": "file", "encoding": "base64", "size": len(content), "content": base64.StdEncoding.EncodeToString([]byte(content))}
		case "/api/http/projects/key:owner/repositories/repo/files":
			ref := r.URL.Query().Get("commit")
			_, exists := fixture.Source[ref]
			if !exists || r.URL.Query().Get("path") != historyFixtureSourcePath {
				http.NotFound(w, r)
				return
			}
			value = []any{map[string]any{"path": historyFixtureSourcePath, "type": "FILE", "blob": ref}}
		case "/api/http/projects/key:owner/repositories/repo/content":
			content, exists := fixture.Source[r.URL.Query().Get("blobId")]
			if !exists {
				http.NotFound(w, r)
				return
			}
			value = map[string]any{"totalSize": len(content), "partBase64": base64.StdEncoding.EncodeToString([]byte(content))}
		case "/api/http/projects/key:owner/code-reviews/number:7":
			value = map[string]any{"id": "review-id", "number": 7, "state": "Closed", "timestamp": int64(1790000000000), "title": fixture.Title, "description": "Correct worker synchronization", "feedChannelId": "feed", "branchPair": map[string]any{"repository": "repo", "isMerged": true, "sourceBranchRef": historyFixtureFixRevision, "targetBranchInfo": map[string]string{"ref": historyFixtureSnapshot}}}
		case "/api/http/chats/messages/sync-batch":
			value = map[string]any{"data": []any{map[string]any{"chatMessage": map[string]any{"id": "root", "projectedItem": map[string]any{"author": map[string]any{"name": "Reviewer", "details": map[string]any{"user": map[string]string{"id": "person"}}}}, "details": map[string]any{"className": "CodeDiscussionAddedFeedEvent", "codeDiscussion": map[string]any{"id": "discussion", "channel": map[string]string{"id": "thread"}, "anchor": map[string]any{"filename": historyFixtureSourcePath, "line": 5, "revision": historyFixtureSnapshot}}}}}}, "etag": "done", "hasMore": false}
		case "/api/http/chats/messages":
			value = map[string]any{"messages": []any{map[string]any{"id": "comment", "text": fixture.Message, "author": map[string]string{"name": "Reviewer"}, "created": map[string]string{"iso": "2026-09-20T12:00:00Z"}}}, "orgLimitReached": false, "nextStartFromDate": nil}
		default:
			t.Errorf("unexpected review API request %s", r.URL)
			http.NotFound(w, r)
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(value))
	}))
	t.Cleanup(server.Close)
	if provider == "github" {
		t.Setenv("EDICT_GITHUB_API_URL", server.URL)
		t.Setenv("GITHUB_TOKEN", "fixture-review-token")
		fixture.URL = "https://github.test/owner/repo/pull/7#discussion_r10"
	} else {
		t.Setenv("EDICT_SPACE_URL", server.URL)
		t.Setenv("SPACE_TOKEN", "fixture-review-token")
		fixture.URL = server.URL + "/im/review/review-id?channel=feed&message=root"
	}
	return fixture
}

func assertManagedReviewSignals(t *testing.T, project managedTestProject, fixture managedReviewFixture) {
	t.Helper()
	paths, err := project.Store.List("inbox")
	require.NoError(t, err)
	require.Len(t, paths, 2, "review correction must retain both evidence sides")
	expected := historyFixSignalExpectation()
	diff := runCommand(t, project.Checkout.RepositoryDirectory, project.Checkout.GitBinary, "--no-pager", "diff", "--no-color", "--no-ext-diff", "--unified=200", expected.Parent, expected.Commit, "--", expected.Path, expected.Path)
	seen := map[string]bool{}
	for _, name := range paths {
		file, err := project.Store.Read(name)
		require.NoError(t, err)
		var signal struct {
			ID             string                    `json:"id"`
			IdempotencyKey string                    `json:"idempotencyKey"`
			Label          string                    `json:"label"`
			FileRevision   qodanaHistoryFileRevision `json:"fileRevision"`
			Source         struct {
				Type               string   `json:"type"`
				PRNumber           int      `json:"prNumber"`
				Title              string   `json:"title"`
				DiscussionMessages []string `json:"discussionMessages"`
				URL                string   `json:"url"`
				Diff               string   `json:"diffPositiveToNegative"`
			} `json:"source"`
			Provenance struct {
				WorkItemID string `json:"workItemId"`
				BatchID    string `json:"analysisBatchId"`
			} `json:"provenance"`
		}
		require.NoError(t, json.Unmarshal([]byte(file.Content), &signal))
		digest := sha256.Sum256([]byte(signal.IdempotencyKey))
		assert.Equal(t, "s-"+hex.EncodeToString(digest[:])[:10], signal.ID, "%s ID", name)
		assert.Equal(t, "inbox/"+signal.ID+".json", name)
		assert.Equal(t, "FromPR", signal.Source.Type, "%s source", name)
		assert.Equal(t, 7, signal.Source.PRNumber)
		assert.Equal(t, fixture.Title, signal.Source.Title)
		assert.Equal(t, []string{fixture.Message}, signal.Source.DiscussionMessages)
		assert.Equal(t, fixture.URL, signal.Source.URL)
		// Local Git includes object/mode headers. Ultimate-style remote file_diff
		// returns canonical file headers and hunks. Both must match real Git bytes.
		providerDiff := diff[strings.Index(diff, "--- a/"):]
		assert.Contains(t, []string{diff, providerDiff}, signal.Source.Diff, "%s canonical diff", name)
		assert.Equal(t, expected.Path, signal.FileRevision.Path)
		assert.NotEmpty(t, signal.Provenance.BatchID)
		assert.True(t, strings.HasPrefix(signal.Provenance.WorkItemID, "pr-7-"))
		assert.False(t, seen[signal.Label], "duplicate evidence label")
		seen[signal.Label] = true
		line, revision := 5, expected.Parent
		if signal.Label == "NEGATIVE" {
			line, revision = 10, expected.Commit
		} else {
			assert.Equal(t, "POSITIVE", signal.Label)
		}
		assert.Equal(t, revision, signal.FileRevision.Revision, "%s evidence revision", name)
		require.NotEmpty(t, signal.FileRevision.ExpectedRanges)
		coversCorrection := false
		for _, r := range signal.FileRevision.ExpectedRanges {
			assert.GreaterOrEqual(t, r.Start, 1, "%s range start", name)
			assert.GreaterOrEqual(t, r.End, r.Start, "%s range end", name)
			assert.LessOrEqual(t, r.End, strings.Count(fixture.Source[revision], "\n"), "%s range exceeds historical source", name)
			coversCorrection = coversCorrection || (r.Start <= line && line <= r.End)
		}
		// The corrected declaration may be supporting evidence in addition to the
		// await call. Require the correction itself without forbidding that range.
		assert.True(t, coversCorrection, "%s must cover the actual correction at line %d", name, line)
	}
}

func assertManagedReviewCalls(t *testing.T, root string) {
	t.Helper()
	file, err := os.Open(filepath.Join(root, "log", "edict", "edict-mcp-system.log"))
	require.NoError(t, err)
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	calls := map[string]int{}
	pending := map[int]string{}
	for scanner.Scan() {
		var event struct {
			Message   string `json:"msg"`
			Method    string `json:"method"`
			RequestID int    `json:"requestId"`
			Params    struct {
				Name string `json:"name"`
			} `json:"params"`
			Result struct {
				IsError bool `json:"isError"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
			Error string `json:"error"`
		}
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &event))
		if event.Method != "tools/call" {
			continue
		}
		if event.Message == "request" {
			calls[event.Params.Name]++
			pending[event.RequestID] = event.Params.Name
		} else if event.Message == "response" {
			name := pending[event.RequestID]
			delete(pending, event.RequestID)
			var detail string
			for _, content := range event.Result.Content {
				detail += content.Text
			}
			// Checking whether an idempotent output already exists can legitimately
			// return ENOENT. Provider, validation, lifecycle and write failures cannot.
			missingOutput := name == "edict_read" && strings.Contains(detail, "no such file or directory")
			assert.False(t, event.Result.IsError && !missingOutput, "%s failed: %s; inspect %s", name, detail, root)
			assert.Empty(t, event.Error)
		}
	}
	require.NoError(t, scanner.Err())
	for _, name := range []string{"edict_prepare_pr_analysis", "edict_list_pr_analysis_items", "edict_get_pr_analysis_item", "edict_validate_pr_signals"} {
		assert.Positive(t, calls[name], "missing real %s call", name)
	}
}
