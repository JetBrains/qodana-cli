// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type signalRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type signalRecord struct {
	ID             string `json:"id"`
	IdempotencyKey string `json:"idempotencyKey"`
	Label          string `json:"label"`
	Description    string `json:"description"`
	FileRevision   struct {
		Path           string        `json:"path"`
		Revision       string        `json:"revision"`
		ExpectedRanges []signalRange `json:"expectedRanges"`
	} `json:"fileRevision"`
	Source struct {
		Type                   string            `json:"type"`
		CommitRevision         string            `json:"commitRevision"`
		ParentRevision         string            `json:"parentRevision"`
		Message                string            `json:"message"`
		PRNumber               int               `json:"prNumber"`
		Title                  string            `json:"title"`
		DiscussionMessages     []json.RawMessage `json:"discussionMessages"`
		URL                    string            `json:"url"`
		DiffPositiveToNegative string            `json:"diffPositiveToNegative"`
	} `json:"source"`
	Provenance struct {
		WorkItemID string `json:"workItemId"`
	} `json:"provenance"`
}

var fullRevision = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// Check the signal contract and consistency with the supplied evidence before
// persistence. Source authenticity and semantic relevance still require review
// against the actual repository/provider; the state store has neither.
func validateSignal(name, content string) error {
	invalid := func(field, reason string) error {
		return fmt.Errorf("invalid signal %q: %s: %s", name, field, reason)
	}
	var signal signalRecord
	if err := json.Unmarshal([]byte(content), &signal); err != nil {
		return invalid("JSON", err.Error())
	}
	for _, field := range []struct{ name, value string }{
		{"idempotencyKey", signal.IdempotencyKey}, {"description", signal.Description},
		{"provenance.workItemId", signal.Provenance.WorkItemID},
		{"source.diffPositiveToNegative", signal.Source.DiffPositiveToNegative},
	} {
		if strings.TrimSpace(field.value) == "" {
			return invalid(field.name, "must not be empty")
		}
	}
	expectedID := "s-" + hash(signal.IdempotencyKey)[:10]
	if signal.ID != expectedID {
		return invalid("id", fmt.Sprintf("got %q; expected %q from SHA-256 of idempotencyKey", signal.ID, expectedID))
	}
	if path.Base(name) != signal.ID+".json" {
		return invalid("id", fmt.Sprintf("filename must be %q", signal.ID+".json"))
	}
	if signal.Label != "POSITIVE" && signal.Label != "NEGATIVE" {
		return invalid("label", "must be POSITIVE (before) or NEGATIVE (after)")
	}
	file := signal.FileRevision
	if file.Path == "" || file.Path == "." || file.Path == ".." || path.IsAbs(file.Path) || path.Clean(file.Path) != file.Path ||
		strings.HasPrefix(file.Path, "../") || strings.ContainsAny(file.Path, "\\\x00") {
		return invalid("fileRevision.path", "must be a clean path relative to the repository root")
	}
	if !fullRevision.MatchString(file.Revision) {
		return invalid("fileRevision.revision", "must be a full Git revision (40 or 64 lowercase hex characters)")
	}
	if len(file.ExpectedRanges) == 0 {
		return invalid("fileRevision.expectedRanges", "must contain at least one {start, end} range")
	}
	for i, r := range file.ExpectedRanges {
		if r.Start < 1 || r.End < r.Start {
			return invalid(fmt.Sprintf("fileRevision.expectedRanges[%d]", i), "requires integer start and end with 1 <= start <= end")
		}
	}
	source := signal.Source
	switch source.Type {
	case "FromCommit":
		if !strings.HasSuffix(source.DiffPositiveToNegative, "\n") {
			return invalid("source.diffPositiveToNegative", "must preserve the canonical Git diff's final newline; read the exact Git output without trimming it")
		}
		for _, field := range []struct{ name, value string }{
			{"source.commitRevision", source.CommitRevision}, {"source.parentRevision", source.ParentRevision},
		} {
			if !fullRevision.MatchString(field.value) {
				return invalid(field.name, "must be a full Git revision (40 or 64 lowercase hex characters)")
			}
		}
		if strings.TrimSpace(source.Message) == "" {
			return invalid("source.message", "must not be empty")
		}
		revision := source.ParentRevision
		if signal.Label == "NEGATIVE" {
			revision = source.CommitRevision
		}
		if file.Revision != revision {
			return invalid("fileRevision.revision", fmt.Sprintf("got %q; expected %q for %s evidence", file.Revision, revision, signal.Label))
		}
	case "FromPR":
		if source.PRNumber < 1 || strings.TrimSpace(source.Title) == "" || len(source.DiscussionMessages) == 0 || strings.TrimSpace(source.URL) == "" {
			return invalid("source", "FromPR requires prNumber > 0, title, discussionMessages, and url")
		}
	default:
		return invalid("source.type", "must be FromCommit or FromPR")
	}
	changed, err := signalDiffLines(source.DiffPositiveToNegative, signal.Label)
	if err != nil {
		return invalid("source.diffPositiveToNegative", err.Error())
	}
	lines, ok := changed[file.Path]
	if !ok {
		paths := make([]string, 0, len(changed))
		for name := range changed {
			paths = append(paths, name)
		}
		slices.Sort(paths)
		return invalid("fileRevision.path", fmt.Sprintf("got %q; expected a repository-relative %s path from the diff: %q", file.Path, signal.Label, paths))
	}
	for i, r := range file.ExpectedRanges {
		if !slices.ContainsFunc(lines, func(line int) bool { return r.Start <= line && line <= r.End }) {
			return invalid(fmt.Sprintf("fileRevision.expectedRanges[%d]", i), fmt.Sprintf("%d:%d must intersect a changed %s line in the diff for %q", r.Start, r.End, signal.Label, file.Path))
		}
	}
	return nil
}

var signalDiffHunk = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

// Read both sides of ordinary unified Git diffs, including renames and quoted
// paths. Hunk counts distinguish content such as "--- x" from file headers.
func signalDiffLines(diff, label string) (map[string][]int, error) {
	changed := make(map[string][]int)
	var before, after string
	var oldLine, newLine, oldRemaining, newRemaining int
	for number, line := range strings.Split(diff, "\n") {
		bad := func() (map[string][]int, error) {
			return nil, fmt.Errorf("malformed unified diff at line %d", number+1)
		}
		if strings.HasPrefix(line, "\\ No newline at end of file") {
			continue
		}
		if oldRemaining > 0 || newRemaining > 0 {
			if line == "" {
				return bad()
			}
			switch line[0] {
			case ' ':
				oldRemaining--
				newRemaining--
				oldLine++
				newLine++
			case '-':
				if label == "POSITIVE" && before != "/dev/null" {
					changed[before] = append(changed[before], oldLine)
				}
				oldRemaining--
				oldLine++
			case '+':
				if label == "NEGATIVE" && after != "/dev/null" {
					changed[after] = append(changed[after], newLine)
				}
				newRemaining--
				newLine++
			default:
				return bad()
			}
			if oldRemaining < 0 || newRemaining < 0 {
				return bad()
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "diff --git "):
			before, after = "", ""
		case strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "):
			name := strings.TrimSuffix(line[4:], "\t")
			if strings.HasPrefix(name, `"`) {
				var err error
				name, err = strconv.Unquote(name)
				if err != nil {
					return bad()
				}
			}
			if strings.HasPrefix(line, "--- ") {
				before = strings.TrimPrefix(name, "a/")
			} else {
				after = strings.TrimPrefix(name, "b/")
			}
		case strings.HasPrefix(line, "@@"):
			parts := signalDiffHunk.FindStringSubmatch(line)
			if parts == nil || before == "" || after == "" {
				return bad()
			}
			values := []int{0, 1, 0, 1}
			for i, part := range parts[1:] {
				if part != "" {
					var err error
					values[i], err = strconv.Atoi(part)
					if err != nil {
						return bad()
					}
				}
			}
			oldLine, oldRemaining, newLine, newRemaining = values[0], values[1], values[2], values[3]
		}
	}
	if oldRemaining != 0 || newRemaining != 0 {
		return nil, fmt.Errorf("truncated unified diff")
	}
	if len(changed) == 0 {
		return nil, fmt.Errorf("no changed %s lines in a unified diff", label)
	}
	return changed, nil
}
