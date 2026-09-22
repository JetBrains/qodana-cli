// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/JetBrains/qodana-cli/edict/managed/review"
)

const prSkill = "edict-next-pr-signal-analysis"

type prItem struct {
	WorkItemID string            `json:"workItemId"`
	SourceType string            `json:"sourceType"`
	Repository review.Repository `json:"repository"`
	PR         review.PR         `json:"pr"`
	Thread     review.Thread     `json:"thread"`
}

type prBatchSummary struct {
	BatchID              string `json:"batchId"`
	SelectedPRCount      int    `json:"selectedPrCount"`
	PRCountWithWorkItems int    `json:"prCountWithWorkItems"`
	TotalWorkItemCount   int    `json:"totalWorkItemCount"`
}

type prBatch struct {
	Summary   prBatchSummary
	Owner     string
	Items     []prItem
	Validated map[string]string // Signal ID to exact validated content hash.
}

type prItemSummary struct {
	WorkItemID   string `json:"workItemId"`
	PRNumber     int    `json:"prNumber"`
	ThreadID     string `json:"threadId"`
	Path         string `json:"path"`
	MessageCount int    `json:"messageCount"`
}

type prPage struct {
	TotalWorkItemCount int             `json:"totalWorkItemCount"`
	Offset             int             `json:"offset"`
	Items              []prItemSummary `json:"items"`
	NextOffset         *int            `json:"nextOffset,omitempty"`
}

type prReceipt struct {
	BatchID                string            `json:"batchId"`
	InspectedWorkItemCount int               `json:"inspectedWorkItemCount"`
	SignalCount            int               `json:"signalCount"`
	Signals                map[string]string `json:"signals"` // State path to exact validated content hash.
}

// Called under the store lock. Leaf readers inherit only their parent's batch.
func (s *Store) prOwner(token string, coordinator bool) (string, error) {
	c, err := s.authorize(token)
	if err != nil {
		return "", err
	}
	if c.skill == prSkill {
		return c.taskID, nil
	}
	if !coordinator && c.skill == "edict-next-signal-analysis" {
		parent := findTask(s.plan, findTask(s.plan, c.taskID).ParentID)
		if parent != nil && parent.Skill == prSkill {
			return parent.ID, nil
		}
	}
	role := prSkill + " task"
	if !coordinator {
		role += " or its signal-analysis worker"
	}
	return "", fmt.Errorf("PR analysis requires a running %s", role)
}

func (s *Store) preparePR(ctx context.Context, token string, selection review.Selection) (prBatchSummary, error) {
	s.mu.Lock()
	_, err := s.prOwner(token, true)
	s.mu.Unlock()
	if err != nil {
		return prBatchSummary{}, err
	}
	prs, err := s.reviewClient.Fetch(ctx, selection)
	if err != nil {
		return prBatchSummary{}, err
	}
	batch := &prBatch{Items: []prItem{}}
	batch.Summary.SelectedPRCount = len(prs)
	seen := map[string]bool{}
	for _, pr := range prs {
		threads := pr.Threads
		pr.Threads = nil
		if len(threads) > 0 {
			batch.Summary.PRCountWithWorkItems++
		}
		for _, thread := range threads {
			identity, _ := json.Marshal([]any{selection.Repository, pr.URL, pr.Number, thread.ID, thread.Path, thread.AnchorLine, thread.AnchorEndLine})
			id := fmt.Sprintf("pr-%d-%s", pr.Number, hash(string(identity))[:16])
			if seen[id] {
				return prBatchSummary{}, fmt.Errorf("duplicate provider work item %s", id)
			}
			seen[id] = true
			batch.Items = append(batch.Items, prItem{id, "PR_DISCUSSION", selection.Repository, pr, thread})
		}
	}
	// Include the actual evidence: an edited discussion must not alias an older snapshot.
	identity, _ := json.Marshal(struct {
		Selection review.Selection
		PRs       []review.PR
	}{selection, prs})
	batch.Summary.BatchID = hash(string(identity))[:24]
	batch.Summary.TotalWorkItemCount = len(batch.Items)
	s.mu.Lock()
	defer s.mu.Unlock()
	owner, err := s.prOwner(token, true)
	if err != nil {
		return prBatchSummary{}, err
	}
	batch.Owner = owner
	key := owner + ":" + batch.Summary.BatchID
	if existing := s.prBatches[key]; existing != nil {
		return existing.Summary, nil
	}
	s.prBatches[key] = batch
	return batch.Summary, nil
}

func (s *Store) prBatchFor(token, batchID string, coordinator bool) (*prBatch, error) {
	owner, err := s.prOwner(token, coordinator)
	if err != nil {
		return nil, err
	}
	batch := s.prBatches[owner+":"+batchID]
	if batch == nil {
		return nil, errors.New("unknown PR batch for this task; prepare it again after a server restart")
	}
	return batch, nil
}

func (s *Store) listPR(token, batchID string, offset, limit int) (prPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	batch, err := s.prBatchFor(token, batchID, false)
	if err != nil {
		return prPage{}, err
	}
	if offset < 0 || offset > len(batch.Items) || limit < 1 || limit > 20 {
		return prPage{}, errors.New("offset must be within the batch and limit must be 1..20")
	}
	end := min(offset+limit, len(batch.Items))
	page := prPage{TotalWorkItemCount: len(batch.Items), Offset: offset, Items: []prItemSummary{}}
	for _, item := range batch.Items[offset:end] {
		page.Items = append(page.Items, prItemSummary{item.WorkItemID, item.PR.Number, item.Thread.ID, item.Thread.Path, len(item.Thread.Messages)})
	}
	if end < len(batch.Items) {
		page.NextOffset = &end
	}
	return page, nil
}

func (s *Store) getPR(token, batchID, itemID string) (prItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	batch, err := s.prBatchFor(token, batchID, false)
	if err != nil {
		return prItem{}, err
	}
	for _, item := range batch.Items {
		if item.WorkItemID == itemID {
			return item, nil
		}
	}
	return prItem{}, fmt.Errorf("unknown work item %q in PR batch", itemID)
}

func (s *Store) validatePR(token, batchID string, inspected []string, contents []string) (prReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	batch, err := s.prBatchFor(token, batchID, true)
	if err != nil {
		return prReceipt{}, err
	}
	ids := make([]string, 0, len(batch.Items))
	items := map[string]prItem{}
	for _, item := range batch.Items {
		ids = append(ids, item.WorkItemID)
		items[item.WorkItemID] = item
	}
	if !slices.Equal(ids, inspected) {
		return prReceipt{}, fmt.Errorf("incomplete or unordered PR coverage: expected exactly %v; received %v", ids, inspected)
	}
	validated := map[string]string{}
	receipt := prReceipt{BatchID: batchID, InspectedWorkItemCount: len(ids), Signals: map[string]string{}}
	for index, content := range contents {
		if len(content) > maxFileSize {
			return prReceipt{}, fmt.Errorf("signal %d exceeds the 8 MiB artifact limit", index)
		}
		var signal signalRecord
		if err := json.Unmarshal([]byte(content), &signal); err != nil {
			return prReceipt{}, fmt.Errorf("signal %d: invalid JSON", index)
		}
		name := "inbox/" + signal.ID + ".json"
		if err := validateSignal(name, content); err != nil {
			return prReceipt{}, err
		}
		item, ok := items[signal.Provenance.WorkItemID]
		if !ok {
			return prReceipt{}, fmt.Errorf("signal %s references unknown work item %s", signal.ID, signal.Provenance.WorkItemID)
		}
		if _, exists := validated[signal.ID]; exists {
			return prReceipt{}, fmt.Errorf("duplicate signal %s", signal.ID)
		}
		if signal.Source.Type != "FromPR" || signal.Source.PRNumber != item.PR.Number || signal.Source.Title != item.PR.Title || signal.Source.URL != item.Thread.URL {
			return prReceipt{}, fmt.Errorf("signal %s source must preserve its prepared PR number, title and discussion URL", signal.ID)
		}
		var messages []string
		for _, raw := range signal.Source.DiscussionMessages {
			var body string
			if json.Unmarshal(raw, &body) != nil {
				return prReceipt{}, fmt.Errorf("signal %s discussionMessages must contain complete message strings", signal.ID)
			}
			messages = append(messages, body)
		}
		var expected []string
		for _, message := range item.Thread.Messages {
			expected = append(expected, message.Body)
		}
		if !slices.Equal(messages, expected) {
			return prReceipt{}, fmt.Errorf("signal %s must preserve all prepared discussion messages in order", signal.ID)
		}
		ref := signal.FileRevision.Revision
		if signal.Label == "NEGATIVE" && ref != item.PR.HeadRevision || signal.Label == "POSITIVE" && ref != item.Thread.Revision && ref != item.PR.BaseRevision {
			return prReceipt{}, fmt.Errorf("signal %s evidence revision does not match its %s PR side", signal.ID, signal.Label)
		}
		validated[signal.ID] = hash(content)
		receipt.Signals[name] = hash(content)
	}
	batch.Validated = validated
	receipt.SignalCount = len(validated)
	return receipt, nil
}

func (s *Store) validatePRWrite(c capability, name, content string) error {
	if operation(name) != "inbox.write" {
		return nil
	}
	var signal signalRecord
	_ = json.Unmarshal([]byte(content), &signal) // Structure was already checked.
	if c.skill == "edict-next-batch-signal-analysis" && signal.Source.Type == "FromPR" {
		return fmt.Errorf("PR signals must be extracted through %s", prSkill)
	}
	if c.skill != prSkill {
		return nil
	}
	for _, batch := range s.prBatches {
		if batch.Owner == c.taskID && batch.Validated[signal.ID] == hash(content) {
			return nil
		}
	}
	return fmt.Errorf("PR signal %s has not passed edict_validate_pr_signals with these exact bytes", signal.ID)
}

func (s *Store) prSourceItem(token, batchID, itemID string, revisions ...string) (prItem, error) {
	item, err := s.getPR(token, batchID, itemID)
	if err != nil {
		return item, err
	}
	for _, revision := range revisions {
		if !slices.Contains([]string{item.PR.BaseRevision, item.PR.HeadRevision, item.Thread.Revision}, revision) {
			return item, errors.New("revision does not belong to the prepared PR work item")
		}
	}
	return item, nil
}

func prSummaryText(batch prBatchSummary) string {
	return fmt.Sprintf("%d merged PRs, %d discussions; batch %s", batch.SelectedPRCount, batch.TotalWorkItemCount, batch.BatchID)
}

func (s *Store) completePR(taskID string) error {
	found := false
	for _, batch := range s.prBatches {
		if batch.Owner != taskID {
			continue
		}
		found = true
		if batch.Validated == nil {
			return errors.New("validate complete PR inspection coverage before finishing")
		}
		for id, expected := range batch.Validated {
			file, err := s.read("inbox/" + id + ".json")
			if err != nil || file.Hash != expected {
				return fmt.Errorf("validated PR signal %s is missing or differs from its receipt", id)
			}
		}
	}
	if !found {
		return errors.New("prepare and inspect the requested PR selection before finishing")
	}
	return nil
}
