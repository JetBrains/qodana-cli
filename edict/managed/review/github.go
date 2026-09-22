// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package review

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

type githubPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	URL       string `json:"html_url"`
	MergedAt  string `json:"merged_at"`
	UpdatedAt string `json:"updated_at"`
	Base      struct {
		SHA string `json:"sha"`
	} `json:"base"`
	Head struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

type githubComment struct {
	ID                int64  `json:"id"`
	ReplyTo           int64  `json:"in_reply_to_id"`
	ReviewID          int64  `json:"pull_request_review_id"`
	Path              string `json:"path"`
	Body              string `json:"body"`
	CreatedAt         string `json:"created_at"`
	Revision          string `json:"original_commit_id"`
	Line              int    `json:"line"`
	OriginalLine      int    `json:"original_line"`
	StartLine         int    `json:"start_line"`
	OriginalStartLine int    `json:"original_start_line"`
	User              struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"user"`
}

type githubReview struct {
	ID          int64  `json:"id"`
	Body        string `json:"body"`
	SubmittedAt string `json:"submitted_at"`
	User        struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"user"`
}

// Page numbers are generated locally: never forward credentials to a Link URL.
func githubPages[T any](ctx context.Context, c *Client, endpoint string, query url.Values) ([]T, error) {
	var all []T
	for page := 1; page <= maxPages; page++ {
		query.Set("per_page", "100")
		query.Set("page", strconv.Itoa(page))
		var items []T
		headers, err := c.get(ctx, "github", endpoint, query, &items)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if !strings.Contains(headers.Get("Link"), `rel="next"`) && len(items) < 100 {
			return all, nil
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("GitHub pagination advertised another page without returning items")
		}
	}
	return nil, fmt.Errorf("GitHub pagination exceeded %d pages; refusing incomplete evidence", maxPages)
}

func (c *Client) github(ctx context.Context, selection Selection) ([]PR, error) {
	numbers := slices.Clone(selection.PRNumbers)
	root := repoEndpoint(selection.Repository)
	if len(numbers) == 0 {
		// List by update time, then filter by merge time. Never stop merely because
		// one old PR was recently updated: that says nothing about the following PRs.
		start, _ := time.Parse(time.DateOnly, selection.StartDate)
		seen := map[int]bool{}
		for page := 1; ; page++ {
			if page > maxPages {
				return nil, fmt.Errorf("GitHub PR discovery exceeded %d pages", maxPages)
			}
			var items []githubPR
			headers, err := c.get(ctx, "github", root+"/pulls", url.Values{"state": {"closed"}, "sort": {"updated"}, "direction": {"desc"}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}, &items)
			if err != nil {
				return nil, err
			}
			old := false
			for _, item := range items {
				if item.Number < 1 || seen[item.Number] {
					return nil, fmt.Errorf("GitHub discovery returned an invalid or repeated PR number")
				}
				seen[item.Number] = true
				updated, err := time.Parse(time.RFC3339, item.UpdatedAt)
				if err != nil {
					return nil, fmt.Errorf("PR %d has no valid updated_at", item.Number)
				}
				if updated.Before(start) {
					old = true
					break
				}
				if item.MergedAt == "" {
					continue
				}
				merged, err := time.Parse(time.RFC3339, item.MergedAt)
				if err != nil {
					return nil, fmt.Errorf("PR %d has invalid merged_at", item.Number)
				}
				if selection.containsDate(merged.UnixMilli()) {
					numbers = append(numbers, item.Number)
				}
				if len(numbers) == selection.MaxPRs {
					break
				}
			}
			if old || len(numbers) == selection.MaxPRs || (len(items) < 100 && !strings.Contains(headers.Get("Link"), `rel="next"`)) {
				break
			}
			if len(items) == 0 {
				return nil, fmt.Errorf("GitHub PR discovery returned an empty nonterminal page")
			}
		}
	}
	prs := make([]PR, 0, len(numbers))
	for _, number := range numbers {
		var value githubPR
		endpoint := root + "/pulls/" + strconv.Itoa(number)
		if _, err := c.get(ctx, "github", endpoint, nil, &value); err != nil {
			return nil, fmt.Errorf("PR %d: %w", number, err)
		}
		if value.Number != number {
			return nil, fmt.Errorf("GitHub returned the wrong PR for %d", number)
		}
		if value.MergedAt == "" {
			continue
		}
		merged, err := time.Parse(time.RFC3339, value.MergedAt)
		if err != nil {
			return nil, fmt.Errorf("PR %d has invalid merged_at", number)
		}
		if !selection.containsDate(merged.UnixMilli()) {
			continue
		}
		comments, err := githubPages[githubComment](ctx, c, endpoint+"/comments", url.Values{})
		if err != nil {
			return nil, fmt.Errorf("PR %d comments: %w", number, err)
		}
		reviews, err := githubPages[githubReview](ctx, c, endpoint+"/reviews", url.Values{})
		if err != nil {
			return nil, fmt.Errorf("PR %d reviews: %w", number, err)
		}
		pr := PR{Number: number, URL: value.URL, Title: value.Title, Body: value.Body, BaseRevision: value.Base.SHA, HeadRevision: value.Head.SHA, CloseTimestamp: merged.UnixMilli()}
		pr.Threads, err = githubThreads(pr, comments, reviews)
		if err != nil {
			return nil, err
		}
		prs = append(prs, pr)
	}
	return prs, nil
}

func githubThreads(pr PR, comments []githubComment, reviews []githubReview) ([]Thread, error) {
	byID := map[int64]githubComment{}
	groups := map[int64][]githubComment{}
	for _, comment := range comments {
		if comment.ID <= 0 {
			return nil, fmt.Errorf("PR %d contains a comment without an ID", pr.Number)
		}
		if _, exists := byID[comment.ID]; exists {
			return nil, fmt.Errorf("PR %d contains duplicate comment %d", pr.Number, comment.ID)
		}
		byID[comment.ID] = comment
		root := comment.ReplyTo
		if root == 0 {
			root = comment.ID
		}
		groups[root] = append(groups[root], comment)
	}
	var threads []Thread
	for id, group := range groups {
		root, exists := byID[id]
		if !exists {
			return nil, fmt.Errorf("PR %d is missing root comment %d", pr.Number, id)
		}
		line := root.OriginalLine
		if line == 0 {
			line = root.Line
		}
		start := root.OriginalStartLine
		if start == 0 {
			start = root.StartLine
		}
		if start == 0 {
			start = line
		}
		thread := Thread{ID: fmt.Sprintf("github-%d-%d", pr.Number, id), URL: fmt.Sprintf("%s#discussion_r%d", pr.URL, id), Path: root.Path, Revision: root.Revision, AnchorLine: start, AnchorEndLine: line}
		for _, review := range reviews {
			if review.ID == root.ReviewID && strings.TrimSpace(review.Body) != "" && !isBot(review.User.Login, review.User.Type) {
				thread.Messages = append(thread.Messages, Message{review.User.Login, review.Body, review.SubmittedAt})
			}
		}
		slices.SortFunc(group, func(a, b githubComment) int {
			if v := strings.Compare(a.CreatedAt, b.CreatedAt); v != 0 {
				return v
			}
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		})
		for _, comment := range group {
			if !isBot(comment.User.Login, comment.User.Type) {
				thread.Messages = append(thread.Messages, Message{comment.User.Login, comment.Body, comment.CreatedAt})
			}
		}
		if len(thread.Messages) > 0 {
			threads = append(threads, thread)
		}
	}
	slices.SortFunc(threads, func(a, b Thread) int {
		if v := strings.Compare(a.Messages[0].CreatedAt, b.Messages[0].CreatedAt); v != 0 {
			return v
		}
		return strings.Compare(a.ID, b.ID)
	})
	return threads, nil
}
