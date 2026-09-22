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

type spaceReview struct {
	ID            string `json:"id"`
	Number        int    `json:"number"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	State         string `json:"state"`
	Timestamp     int64  `json:"timestamp"`
	FeedChannelID string `json:"feedChannelId"`
	BranchPair    struct {
		Repository string `json:"repository"`
		IsMerged   *bool  `json:"isMerged"`
		Source     string `json:"sourceBranchRef"`
		Target     struct {
			Ref string `json:"ref"`
		} `json:"targetBranchInfo"`
	} `json:"branchPair"`
}

type spaceAuthor struct {
	Name    string `json:"name"`
	Details struct {
		ClassName string `json:"className"`
		User      *struct {
			ID string `json:"id"`
		} `json:"user"`
	} `json:"details"`
}

type spaceMessage struct {
	ID      string      `json:"id"`
	Text    string      `json:"text"`
	Author  spaceAuthor `json:"author"`
	Created struct {
		ISO       string `json:"iso"`
		Timestamp int64  `json:"timestamp"`
	} `json:"created"`
	ProjectedItem struct {
		Author spaceAuthor `json:"author"`
	} `json:"projectedItem"`
	Details struct {
		ClassName  string `json:"className"`
		Discussion *struct {
			ID      string `json:"id"`
			Channel struct {
				ID string `json:"id"`
			} `json:"channel"`
			Anchor struct {
				Filename string `json:"filename"`
				Line     int    `json:"line"`
				OldLine  int    `json:"oldLine"`
				Revision string `json:"revision"`
			} `json:"anchor"`
		} `json:"codeDiscussion"`
	} `json:"details"`
}

func (c *Client) space(ctx context.Context, selection Selection) ([]PR, error) {
	numbers := slices.Clone(selection.PRNumbers)
	root := "/projects/key:" + url.PathEscape(selection.Owner) + "/code-reviews"
	if len(numbers) == 0 {
		start, _ := time.Parse(time.DateOnly, selection.StartDate)
		seen := map[int]bool{}
		for page := 0; ; page++ {
			if page >= maxPages {
				return nil, fmt.Errorf("Space PR discovery exceeded %d pages", maxPages)
			}
			var response struct {
				Data []struct {
					Review spaceReview `json:"review"`
				} `json:"data"`
			}
			_, err := c.get(ctx, "space", root, url.Values{"$skip": {strconv.Itoa(page * 100)}, "$top": {"100"}, "state": {"Merged"}, "repository": {selection.Repo}, "sort": {"LastUpdatedDesc"}, "to": {selection.EndDate}, "$fields": {"data(review(id,number,timestamp))"}}, &response)
			if err != nil {
				return nil, err
			}
			old := false
			for _, item := range response.Data {
				r := item.Review
				if r.Number < 1 || seen[r.Number] {
					return nil, fmt.Errorf("Space discovery returned an invalid or repeated review number")
				}
				seen[r.Number] = true
				if r.Timestamp <= 0 {
					return nil, fmt.Errorf("Space review %d lacks its timestamp", r.Number)
				}
				if r.Timestamp < start.UnixMilli() {
					old = true
					break
				}
				if selection.containsDate(r.Timestamp) {
					numbers = append(numbers, r.Number)
				}
				if len(numbers) == selection.MaxPRs {
					break
				}
			}
			if old || len(numbers) == selection.MaxPRs || len(response.Data) < 100 {
				break
			}
		}
	}
	prs := make([]PR, 0, len(numbers))
	for _, number := range numbers {
		var value spaceReview
		_, err := c.get(ctx, "space", root+"/number:"+strconv.Itoa(number), url.Values{"$fields": {"id,number,state,timestamp,title,description,feedChannelId,branchPair(repository,isMerged,sourceBranchRef,targetBranchInfo(ref))"}}, &value)
		if err != nil {
			return nil, fmt.Errorf("review %d: %w", number, err)
		}
		if value.Number != number {
			return nil, fmt.Errorf("Space returned the wrong review for %d", number)
		}
		if value.State == "" {
			return nil, fmt.Errorf("Space review %d lacks its merge state", number)
		}
		if value.State != "Closed" {
			continue
		}
		if value.BranchPair.Repository != selection.Repo {
			return nil, fmt.Errorf("Space review %d does not belong to repository %q", number, selection.Repo)
		}
		if value.BranchPair.IsMerged == nil {
			return nil, fmt.Errorf("Space review %d lacks branch merge status", number)
		}
		if !*value.BranchPair.IsMerged {
			continue
		}
		if value.FeedChannelID == "" {
			return nil, fmt.Errorf("Space review %d lacks its discussion feed", number)
		}
		feed, err := c.spaceFeed(ctx, value.FeedChannelID)
		if err != nil {
			return nil, fmt.Errorf("review %d feed: %w", number, err)
		}
		pr := PR{Number: number, URL: c.SpaceURL + "/p/" + url.PathEscape(selection.Owner) + "/reviews/" + strconv.Itoa(number), Title: value.Title, Body: value.Description, BaseRevision: value.BranchPair.Target.Ref, HeadRevision: value.BranchPair.Source, CloseTimestamp: value.Timestamp}
		if !selection.containsDate(pr.CloseTimestamp) {
			continue
		}
		for _, message := range feed {
			if message.Details.ClassName != "CodeDiscussionAddedFeedEvent" {
				continue
			}
			author := message.ProjectedItem.Author
			if author.Details.User == nil || isBot(author.Name, author.Details.ClassName) {
				continue
			}
			discussion := message.Details.Discussion
			if discussion == nil || discussion.Channel.ID == "" {
				return nil, fmt.Errorf("review %d discussion %s lacks a channel or anchor", number, message.ID)
			}
			messages, err := c.spaceDiscussion(ctx, discussion.Channel.ID)
			if err != nil {
				return nil, fmt.Errorf("review %d discussion %s: %w", number, message.ID, err)
			}
			if len(messages) == 0 {
				continue
			}
			line := discussion.Anchor.Line
			if line == 0 {
				line = discussion.Anchor.OldLine
			}
			pr.Threads = append(pr.Threads, Thread{ID: fmt.Sprintf("space-%d-%s", number, message.ID), URL: c.SpaceURL + "/im/review/" + url.PathEscape(value.ID) + "?" + url.Values{"message": {message.ID}, "channel": {value.FeedChannelID}}.Encode(), Path: strings.TrimPrefix(discussion.Anchor.Filename, "/"), Revision: discussion.Anchor.Revision, AnchorLine: line, AnchorEndLine: line, Messages: messages})
		}
		slices.SortFunc(pr.Threads, func(a, b Thread) int {
			if v := strings.Compare(a.Messages[0].CreatedAt, b.Messages[0].CreatedAt); v != 0 {
				return v
			}
			return strings.Compare(a.ID, b.ID)
		})
		prs = append(prs, pr)
	}
	return prs, nil
}

func (c *Client) spaceFeed(ctx context.Context, channel string) ([]spaceMessage, error) {
	var all []spaceMessage
	etag := "0"
	seen := map[string]bool{}
	for page := 0; page < maxPages; page++ {
		var response struct {
			Data []struct {
				Message *spaceMessage `json:"chatMessage"`
			} `json:"data"`
			ETag    string `json:"etag"`
			HasMore *bool  `json:"hasMore"`
		}
		_, err := c.get(ctx, "space", "/chats/messages/sync-batch", url.Values{"channel": {"id:" + channel}, "batchInfo": {"{etag:" + etag + ",batchSize:100}"}, "$fields": {"data(chatMessage(id,text,created,details(className,codeDiscussion(id,channel(id),anchor(filename,line,oldLine,revision))),projectedItem(author(name,details(className,user(id)))))),etag,hasMore"}}, &response)
		if err != nil {
			return nil, err
		}
		for _, item := range response.Data {
			if item.Message != nil && !seen[item.Message.ID] {
				all = append(all, *item.Message)
				seen[item.Message.ID] = true
			}
		}
		if response.HasMore == nil {
			return nil, fmt.Errorf("Space feed omitted pagination completeness flag")
		}
		if !*response.HasMore {
			return all, nil
		}
		if response.ETag == "" || response.ETag == etag {
			return nil, fmt.Errorf("Space feed pagination did not advance")
		}
		etag = response.ETag
	}
	return nil, fmt.Errorf("Space feed exceeded %d pages; refusing incomplete evidence", maxPages)
}

func (c *Client) spaceDiscussion(ctx context.Context, channel string) ([]Message, error) {
	var all []Message
	start := ""
	seen := map[string]bool{}
	for page := 0; page < maxPages; page++ {
		var response struct {
			Messages          []spaceMessage `json:"messages"`
			NextStartFromDate *struct {
				Timestamp int64 `json:"timestamp"`
			} `json:"nextStartFromDate"`
			OrgLimitReached *bool `json:"orgLimitReached"`
		}
		query := url.Values{"channel": {"id:" + channel}, "sorting": {"FromOldestToNewest"}, "batchSize": {"50"}, "$fields": {"messages(id,text,author(name,details(className,user(id))),created),nextStartFromDate,orgLimitReached"}}
		if start != "" {
			query.Set("startFromDate", start)
		}
		_, err := c.get(ctx, "space", "/chats/messages", query, &response)
		if err != nil {
			return nil, err
		}
		if response.OrgLimitReached == nil || *response.OrgLimitReached {
			return nil, fmt.Errorf("Space discussion history is unavailable or limited by the organization plan")
		}
		fresh := 0
		for _, message := range response.Messages {
			if message.ID == "" {
				return nil, fmt.Errorf("Space discussion has a message without an ID")
			}
			if seen[message.ID] {
				continue
			}
			seen[message.ID] = true
			fresh++
			if message.Text != "" && !isBot(message.Author.Name, message.Author.Details.ClassName) {
				at := message.Created.ISO
				if at == "" && message.Created.Timestamp > 0 {
					at = time.UnixMilli(message.Created.Timestamp).UTC().Format(time.RFC3339Nano)
				}
				all = append(all, Message{message.Author.Name, message.Text, at})
			}
		}
		if len(response.Messages) < 50 {
			return all, nil
		}
		if fresh == 0 || response.NextStartFromDate == nil {
			return nil, fmt.Errorf("Space discussion pagination did not advance")
		}
		next := time.UnixMilli(response.NextStartFromDate.Timestamp).UTC().Format(time.RFC3339Nano)
		if next == start {
			return nil, fmt.Errorf("Space discussion pagination repeated its date cursor")
		}
		start = next
	}
	return nil, fmt.Errorf("Space discussion exceeded %d pages; refusing incomplete evidence", maxPages)
}
