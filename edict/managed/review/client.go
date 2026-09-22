// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

// Package review reads GitHub and Space review evidence without modifying either provider.
// The normalized packages follow Ultimate's PRAnalysisMcpService and RemoteVcsService contract.
package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
)

const maxPages = 1000
const maxResponse = 16 << 20

type Repository struct {
	Provider string `json:"provider" jsonschema:"github or space"`
	Owner    string `json:"owner" jsonschema:"GitHub owner or Space project key"`
	Repo     string `json:"repo" jsonschema:"Repository name"`
}

type Selection struct {
	Repository
	PRNumbers []int  `json:"prNumbers,omitempty" jsonschema:"Explicit positive PR numbers; mutually exclusive with dates"`
	StartDate string `json:"startDate,omitempty" jsonschema:"Inclusive UTC date YYYY-MM-DD"`
	EndDate   string `json:"endDate,omitempty" jsonschema:"Inclusive UTC date YYYY-MM-DD"`
	MaxPRs    int    `json:"maxPrs" jsonschema:"Bound on selected PRs, from 1 to 1000"`
}

type Message struct {
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

type Thread struct {
	ID            string    `json:"threadId"`
	URL           string    `json:"reviewDiscussionUrl"`
	Path          string    `json:"filePath"`
	Revision      string    `json:"originalCommitSha"`
	AnchorLine    int       `json:"anchorLine"`
	AnchorEndLine int       `json:"anchorEndLine"`
	Messages      []Message `json:"messages"`
}

type PR struct {
	Number         int      `json:"number"`
	URL            string   `json:"url"`
	Title          string   `json:"title"`
	Body           string   `json:"body"`
	BaseRevision   string   `json:"baseRevision"`
	HeadRevision   string   `json:"headRevision"`
	CloseTimestamp int64    `json:"closeTimestamp"`
	Threads        []Thread `json:"threads,omitempty"`
}

// Client configuration is supplied by the server host, never by MCP arguments.
// Credentials therefore never enter tool traffic, task prompts, or state files.
type Client struct {
	HTTP                   *http.Client
	GitHubURL, GitHubToken string
	SpaceURL, SpaceToken   string
}

func FromEnvironment() *Client {
	githubURL := os.Getenv("EDICT_GITHUB_API_URL")
	if githubURL == "" {
		githubURL = "https://api.github.com"
	}
	spaceURL := os.Getenv("EDICT_SPACE_URL")
	if spaceURL == "" {
		spaceURL = "https://jetbrains.team"
	}
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		githubToken = os.Getenv("GH_TOKEN")
	}
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		GitHubURL: strings.TrimRight(githubURL, "/"), GitHubToken: githubToken,
		SpaceURL: strings.TrimRight(spaceURL, "/"), SpaceToken: os.Getenv("SPACE_TOKEN"),
	}
}

var revisionPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func ValidRevision(revision string) bool { return revisionPattern.MatchString(revision) }

func ValidPath(p string) bool {
	return p != "" && p != "." && p != ".." && !path.IsAbs(p) && path.Clean(p) == p &&
		!strings.HasPrefix(p, "../") && !strings.ContainsAny(p, "\\\x00\r\n")
}

func (s Selection) Validate() error {
	if s.Provider != "github" && s.Provider != "space" {
		return errors.New("provider must be github or space")
	}
	for _, name := range []string{s.Owner, s.Repo} {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00\r\n") {
			return errors.New("owner and repo must be nonempty path components")
		}
	}
	if s.MaxPRs < 1 || s.MaxPRs > 1000 {
		return errors.New("maxPrs must be between 1 and 1000")
	}
	if len(s.PRNumbers) > 0 {
		if s.StartDate != "" || s.EndDate != "" {
			return errors.New("PR numbers and date bounds are mutually exclusive")
		}
		seen := map[int]bool{}
		for _, number := range s.PRNumbers {
			if number < 1 || seen[number] {
				return errors.New("prNumbers must be distinct positive numbers")
			}
			seen[number] = true
		}
		if len(s.PRNumbers) > s.MaxPRs {
			return errors.New("explicit PR selection exceeds maxPrs")
		}
		return nil
	}
	start, e1 := time.Parse(time.DateOnly, s.StartDate)
	end, e2 := time.Parse(time.DateOnly, s.EndDate)
	if e1 != nil || e2 != nil || start.After(end) {
		return errors.New("provide both inclusive dates as YYYY-MM-DD with startDate <= endDate")
	}
	return nil
}

func (c *Client) Fetch(ctx context.Context, selection Selection) ([]PR, error) {
	if err := selection.Validate(); err != nil {
		return nil, err
	}
	var prs []PR
	var err error
	if selection.Provider == "github" {
		prs, err = c.github(ctx, selection)
	} else {
		prs, err = c.space(ctx, selection)
	}
	if err != nil {
		return nil, err
	}
	for _, pr := range prs {
		if !ValidRevision(pr.BaseRevision) || !ValidRevision(pr.HeadRevision) {
			return nil, fmt.Errorf("PR %d lacks exact base/head revisions", pr.Number)
		}
		for _, thread := range pr.Threads {
			if !ValidPath(thread.Path) || !ValidRevision(thread.Revision) || thread.AnchorLine < 1 || thread.AnchorEndLine < thread.AnchorLine {
				return nil, fmt.Errorf("PR %d thread %s has invalid path, revision or line anchors", pr.Number, thread.ID)
			}
		}
	}
	if len(selection.PRNumbers) == 0 {
		slices.SortFunc(prs, func(a, b PR) int {
			if a.CloseTimestamp < b.CloseTimestamp {
				return -1
			}
			if a.CloseTimestamp > b.CloseTimestamp {
				return 1
			}
			return a.Number - b.Number
		})
	}
	return prs, nil
}

func (s Selection) containsDate(timestamp int64) bool {
	if len(s.PRNumbers) > 0 {
		return true
	}
	start, _ := time.Parse(time.DateOnly, s.StartDate)
	end, _ := time.Parse(time.DateOnly, s.EndDate)
	return timestamp >= start.UnixMilli() && timestamp < end.AddDate(0, 0, 1).UnixMilli()
}

func (c *Client) get(ctx context.Context, provider, endpoint string, query url.Values, output any) (http.Header, error) {
	base, token := c.GitHubURL, c.GitHubToken
	if provider == "space" {
		base, token = c.SpaceURL+"/api/http", c.SpaceToken
		if token == "" {
			return nil, errors.New("Space access requires SPACE_TOKEN in the edict-mcp server environment")
		}
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("invalid %s API URL in server configuration", provider)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("cannot construct %s API request", provider)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "edict-mcp")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s API request failed (connection or timeout)", provider)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%s API: %w", provider, os.ErrNotExist)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s API returned HTTP %d", provider, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s API response failed", provider)
	}
	if len(data) > maxResponse {
		return nil, fmt.Errorf("%s API response exceeds 16 MiB; refusing incomplete evidence", provider)
	}
	if err := json.Unmarshal(data, output); err != nil {
		return nil, fmt.Errorf("invalid %s API JSON response", provider)
	}
	return response.Header, nil
}

func repoEndpoint(repo Repository) string {
	if repo.Provider == "space" {
		return "/projects/key:" + url.PathEscape(repo.Owner) + "/repositories/" + url.PathEscape(repo.Repo)
	}
	return "/repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Repo)
}

func isBot(name, kind string) bool {
	name = strings.ToLower(name)
	return strings.EqualFold(kind, "bot") || strings.HasSuffix(name, "[bot]") || slices.Contains([]string{"dependabot", "github-actions", "patronus", "space-automation", "jetbrains-bot"}, name)
}
