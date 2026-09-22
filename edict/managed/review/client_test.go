// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package review

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var beforeRef = strings.Repeat("a", 40)
var afterRef = strings.Repeat("b", 40)

func providerFixture(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer provider-secret", r.Header.Get("Authorization"))
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	client := FromEnvironment()
	client.GitHubURL, client.SpaceURL = server.URL, server.URL
	client.GitHubToken, client.SpaceToken = "provider-secret", "provider-secret"
	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	require.NoError(t, json.NewEncoder(w).Encode(value))
}

func githubPRFixture(number int) map[string]any {
	return map[string]any{"number": number, "title": "Replace sleeping", "body": "Full PR context", "html_url": fmt.Sprintf("https://github.test/o/r/pull/%d", number), "merged_at": "2026-09-21T12:00:00Z", "updated_at": "2026-09-22T12:00:00Z", "base": map[string]string{"sha": beforeRef}, "head": map[string]string{"sha": afterRef}}
}

func TestGitHubCompletePaginatedDiscussionAndReviewContext(t *testing.T) {
	calls := map[string]int{}
	client := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		switch r.URL.Path {
		case "/repos/o/r/pulls/7":
			writeJSON(t, w, githubPRFixture(7))
		case "/repos/o/r/pulls/8":
			pr := githubPRFixture(8)
			pr["merged_at"] = nil
			writeJSON(t, w, pr)
		case "/repos/o/r/pulls/7/reviews":
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("Link", `<https://untrusted.test/page>; rel="next"`)
				writeJSON(t, w, []any{map[string]any{"id": 90, "body": "Complete review overview", "user": map[string]string{"login": "reviewer"}, "submitted_at": "2026-09-20T10:00:00Z"}})
			} else {
				writeJSON(t, w, []any{})
			}
		case "/repos/o/r/pulls/7/comments":
			comment := map[string]any{"id": 11, "body": strings.Repeat("full root message ", 100), "path": "src/A.java", "original_commit_id": beforeRef, "original_start_line": 2, "original_line": 4, "start_line": 8, "line": 10, "pull_request_review_id": 90, "user": map[string]string{"login": "reviewer", "type": "User"}, "created_at": "2026-09-20T11:00:00Z"}
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("Link", `<https://untrusted.test/page>; rel="next"`)
				writeJSON(t, w, []any{comment})
			} else {
				comment["id"], comment["in_reply_to_id"], comment["body"], comment["created_at"] = 12, 11, "Fixed it in the final commit", "2026-09-20T12:00:00Z"
				writeJSON(t, w, []any{comment, map[string]any{"id": 13, "in_reply_to_id": 11, "body": "bot noise", "user": map[string]string{"login": "helper[bot]", "type": "Bot"}}})
			}
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	})
	selection := Selection{Repository: Repository{"github", "o", "r"}, PRNumbers: []int{7, 8}, MaxPRs: 2}
	prs, err := client.Fetch(context.Background(), selection)
	require.NoError(t, err)
	require.Len(t, prs, 1)
	require.Len(t, prs[0].Threads, 1)
	thread := prs[0].Threads[0]
	require.Len(t, thread.Messages, 3)
	assert.Equal(t, "Complete review overview", thread.Messages[0].Body)
	assert.Len(t, thread.Messages[1].Body, 1800)
	assert.Equal(t, "Fixed it in the final commit", thread.Messages[2].Body)
	assert.Equal(t, 2, thread.AnchorLine)
	assert.Equal(t, 4, thread.AnchorEndLine)
	assert.Equal(t, 2, calls["/repos/o/r/pulls/7/comments"])
	assert.Equal(t, 2, calls["/repos/o/r/pulls/7/reviews"])
	assert.Equal(t, beforeRef, thread.Revision)
}

func TestGitHubDateDiscoveryUsesMergeDateAndInclusiveEnd(t *testing.T) {
	client := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/pulls":
			old := githubPRFixture(1)
			old["merged_at"] = "2020-01-01T00:00:00Z"
			last := githubPRFixture(2)
			last["merged_at"] = "2026-09-21T23:59:59Z"
			next := githubPRFixture(3)
			next["merged_at"] = "2026-09-22T00:00:00Z"
			writeJSON(t, w, []any{old, last, next})
		case "/repos/o/r/pulls/2":
			writeJSON(t, w, githubPRFixture(2))
		case "/repos/o/r/pulls/2/comments", "/repos/o/r/pulls/2/reviews":
			writeJSON(t, w, []any{})
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	})
	prs, err := client.Fetch(context.Background(), Selection{Repository: Repository{"github", "o", "r"}, StartDate: "2026-09-21", EndDate: "2026-09-21", MaxPRs: 10})
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 2, prs[0].Number)
}

func spaceReviewFixture() map[string]any {
	return map[string]any{"id": "review-id", "number": 7, "state": "Closed", "timestamp": int64(1790000000000), "title": "Wait for completion", "description": "Review context", "feedChannelId": "feed", "branchPair": map[string]any{"repository": "repo", "isMerged": true, "sourceBranchRef": afterRef, "targetBranchInfo": map[string]string{"ref": beforeRef}}}
}

func spaceRootFixture() map[string]any {
	return map[string]any{"id": "root", "projectedItem": map[string]any{"author": map[string]any{"name": "Reviewer", "details": map[string]any{"user": map[string]string{"id": "person"}}}}, "details": map[string]any{"className": "CodeDiscussionAddedFeedEvent", "codeDiscussion": map[string]any{"id": "discussion", "channel": map[string]string{"id": "thread"}, "anchor": map[string]any{"filename": "/src/A.java", "line": 3, "revision": beforeRef}}}}
}

func TestSpacePaginatesFeedAndDiscussion(t *testing.T) {
	feedCalls, messageCalls := 0, 0
	client := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http/projects/key:O/code-reviews/number:7":
			writeJSON(t, w, spaceReviewFixture())
		case "/api/http/chats/messages/sync-batch":
			feedCalls++
			if feedCalls == 1 {
				writeJSON(t, w, map[string]any{"data": []any{}, "etag": "second", "hasMore": true})
			} else {
				assert.Contains(t, r.URL.Query().Get("batchInfo"), "etag:second")
				writeJSON(t, w, map[string]any{"data": []any{map[string]any{"chatMessage": spaceRootFixture()}}, "etag": "done", "hasMore": false})
			}
		case "/api/http/chats/messages":
			messageCalls++
			id, body := "1", strings.Repeat("Review context ", 80)
			if messageCalls == 2 {
				id, body = "51", "Correction confirmed"
				assert.Equal(t, "2026-09-21T10:00:00Z", r.URL.Query().Get("startFromDate"))
			}
			messages := []any{map[string]any{"id": id, "text": body, "author": map[string]string{"name": "Reviewer"}, "created": map[string]string{"iso": "2026-09-21T10:00:00Z"}}}
			if messageCalls == 1 {
				for i := 2; i <= 50; i++ {
					messages = append(messages, map[string]any{"id": fmt.Sprint(i), "text": "More context", "author": map[string]string{"name": "Reviewer"}})
				}
			}
			cursor, _ := time.Parse(time.RFC3339, "2026-09-21T10:00:00Z")
			writeJSON(t, w, map[string]any{"messages": messages, "nextStartFromDate": map[string]int64{"timestamp": cursor.UnixMilli()}, "orgLimitReached": false})
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	})
	prs, err := client.Fetch(context.Background(), Selection{Repository: Repository{"space", "O", "repo"}, PRNumbers: []int{7}, MaxPRs: 1})
	require.NoError(t, err)
	require.Len(t, prs, 1)
	require.Len(t, prs[0].Threads, 1)
	thread := prs[0].Threads[0]
	require.Len(t, thread.Messages, 51)
	assert.Equal(t, "Correction confirmed", thread.Messages[50].Body)
	assert.Equal(t, "space-7-root", thread.ID)
	assert.Equal(t, "src/A.java", thread.Path)
	assert.Equal(t, 2, feedCalls)
	assert.Equal(t, 2, messageCalls)
	assert.Equal(t, "Review context", prs[0].Body)
}

func TestSpaceRejectsIncompletePagination(t *testing.T) {
	for _, payload := range []string{`{"data":[],"hasMore":true,"etag":"0"}`, `{"data":[]}`} {
		t.Run(payload, func(t *testing.T) {
			client := providerFixture(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(payload)) })
			_, err := client.spaceFeed(context.Background(), "feed")
			require.Error(t, err)
		})
	}
}

func TestSpaceRejectsRestrictedDiscussionHistory(t *testing.T) {
	client := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"messages": []any{}, "orgLimitReached": true})
	})
	_, err := client.spaceDiscussion(context.Background(), "discussion")
	require.ErrorContains(t, err, "limited by the organization plan")
}

func TestSpaceDistinguishesClosedAndMergedReviewsAndChecksRepository(t *testing.T) {
	for _, test := range []struct {
		name, repo string
		merged     bool
		wantError  string
	}{
		{"closed without merging", "repo", false, ""},
		{"different repository", "other", true, "does not belong"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/http/projects/key:O/code-reviews/number:7", r.URL.Path)
				value := spaceReviewFixture()
				pair := value["branchPair"].(map[string]any)
				pair["repository"], pair["isMerged"] = test.repo, test.merged
				writeJSON(t, w, value)
			})
			prs, err := client.Fetch(context.Background(), Selection{Repository: Repository{"space", "O", "repo"}, PRNumbers: []int{7}, MaxPRs: 1})
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
			} else {
				require.NoError(t, err)
				require.Empty(t, prs)
			}
		})
	}
}

func TestProviderErrorsAndSecrets(t *testing.T) {
	client := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("provider-secret"))
	})
	_, err := client.Fetch(context.Background(), Selection{Repository: Repository{"github", "o", "r"}, PRNumbers: []int{1}, MaxPRs: 1})
	require.ErrorContains(t, err, "HTTP 401")
	assert.NotContains(t, err.Error(), "provider-secret")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Fetch(ctx, Selection{Repository: Repository{"github", "o", "r"}, PRNumbers: []int{1}, MaxPRs: 1})
	require.ErrorIs(t, err, context.Canceled)
}

func TestSelectionRejectsAmbiguousOrUnboundedInputs(t *testing.T) {
	base := Selection{Repository: Repository{"github", "o", "r"}, PRNumbers: []int{1}, MaxPRs: 1}
	for _, edit := range []func(*Selection){
		func(s *Selection) { s.MaxPRs = 0 }, func(s *Selection) { s.PRNumbers = []int{1, 1}; s.MaxPRs = 2 }, func(s *Selection) { s.PRNumbers = []int{0} },
		func(s *Selection) { s.StartDate = "2026-09-21" }, func(s *Selection) { s.PRNumbers = nil }, func(s *Selection) { s.Owner = ".." },
		func(s *Selection) { s.PRNumbers = nil; s.StartDate = "2026-09-22"; s.EndDate = "2026-09-21" },
	} {
		s := base
		edit(&s)
		require.Error(t, s.Validate())
	}
}

func TestProviderSourceDiffAndTruncation(t *testing.T) {
	for _, provider := range []string{"github", "space"} {
		t.Run(provider, func(t *testing.T) {
			truncated := false
			client := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				ref := r.URL.Query().Get("ref")
				if provider == "space" {
					if strings.HasSuffix(r.URL.Path, "/files") {
						writeJSON(t, w, []any{map[string]string{"path": "src/A.java", "type": "FILE", "blob": r.URL.Query().Get("commit")}})
						return
					}
					ref = r.URL.Query().Get("blobId")
				}
				content := "sleep();\nend();\n"
				if ref == afterRef {
					content = "await();\nend();\n"
				}
				size := len(content)
				if truncated {
					size++
				}
				if provider == "github" {
					writeJSON(t, w, map[string]any{"type": "file", "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(content)), "size": size})
				} else {
					writeJSON(t, w, map[string]any{"partBase64": base64.StdEncoding.EncodeToString([]byte(content)), "totalSize": size})
				}
			})
			repo := Repository{provider, "o", "r"}
			diff, err := client.Diff(context.Background(), repo, beforeRef, afterRef, "src/A.java", "src/A.java")
			require.NoError(t, err)
			assert.Equal(t, "--- a/src/A.java\n+++ b/src/A.java\n@@ -1,2 +1,2 @@\n-sleep();\n+await();\n end();\n", diff)
			truncated = true
			_, err = client.File(context.Background(), repo, beforeRef, "src/A.java")
			require.ErrorContains(t, err, "truncated")
		})
	}
}

func TestProviderDoesNotFollowRedirects(t *testing.T) {
	var leaked bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer target.Close()
	client := providerFixture(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) })
	_, err := client.get(context.Background(), "github", "/test", url.Values{}, &map[string]any{})
	require.ErrorContains(t, err, "HTTP 302")
	assert.False(t, leaked)
	assert.Equal(t, 30*time.Second, client.HTTP.Timeout)
}
