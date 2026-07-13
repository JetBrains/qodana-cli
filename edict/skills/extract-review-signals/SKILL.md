---
name: extract-review-signals
description: >
  Extract static-analysis rule signals (inspection specifications) from CI/CD code review
  discussions (GitHub/GitLab pull requests, Space merge requests) and store them as JSON files in
  .qodana/edict/inbox_signals. Use when asked to "extract signals", "extract rules from PR reviews",
  "mine review comments for inspections", "process reviews for edict", or to fill the edict inbox.
  Requires a VCS/CI-CD access skill or MCP (gh CLI, GitHub MCP, Space MCP, GitLab MCP) to be
  available in the session for fetching PR metadata, review threads, and file contents at revisions.
---

# Extract Review Signals

Analyze code review threads from merged PRs/MRs and, for each review remark that describes an
automatically checkable pattern, produce a **signal**: a self-contained JSON file (an inspection
specification) that a downstream edict pipeline turns into a working inspection.

This skill replicates the extraction phase of the PREdict agent (`PRRulesExtractorAgent` +
`CodexRuleExtractionAgent` from the Qodana IDE plugin): one review thread → one extraction
decision → at most one signal.

## Inputs

- **What to process**: PR/MR number(s), a URL, or a date range / "recently merged" query.
  If the user did not specify, ask or default to PRs merged since the last run.
- **Repository**: the current working directory is assumed to be a checkout of the repository the
  PRs belong to. The output directory is resolved against the repo root.
- **CI/CD access**: this skill does NOT implement provider APIs. Use whatever VCS access is
  available in the session — `gh` CLI, GitHub MCP tools, Space/GitLab MCPs, or a dedicated data
  gathering skill. Local `git` is a valid fallback for file contents and diffs when the commits
  are present locally (`git show <sha>:<path>`, `git diff <a> <b> -- <path>`).

## Output

Write one JSON file per extracted signal to:

```
.qodana/edict/inbox_signals/<ruleId>.json
```

- Create the directory if missing.
- On filename collision with a *different* signal, use `<ruleId>-2.json`, `<ruleId>-3.json`, …
  If the existing file describes the *same* rule from another thread, prefer merging the new
  positive/negative examples into `optionalPositiveExamples` / `optionalNegativeExamples` of the
  existing file instead of duplicating the rule.
- Threads that yield no rule produce **no file** in the inbox. Optionally record decisions in
  `.qodana/edict/traces/<pr>-<threadId>-{success|norule}.txt` for auditability.

The signal JSON format is specified in [references/signal-schema.md](references/signal-schema.md).
Follow it exactly — downstream tooling deserializes these files with a strict schema.

## Workflow

### 1. Gather the PR summary

For each PR/MR, collect (via the available CI/CD skill/MCP):

- `number`, `title`, `body`, web `url`
- `baseSha` (merge base / target branch state) and `headSha` (last commit of the PR)
- `closeTimestamp` — merge/close time in epoch **milliseconds**
- **Review threads**: inline review conversations, each with:
  - `filePath` the comment is anchored to
  - `originalCommitSha` — the commit the comment was made on (GitHub: `original_commit_id`)
  - `anchorLine` (and `anchorEndLine` for multi-line comments), 1-based
  - all messages in the thread: author, body, timestamp
  - a stable `threadId` (`<provider>-<prNumber>-<rootCommentId>`, e.g. `github-123-987654`)
  - `reviewDiscussionUrl` — deep link to the thread (GitHub: `<prUrl>#discussion_r<id>`)

Skip PRs that were closed without merging. Skip threads on non-code files unless the remark is
clearly automatable.

### 2. Build per-thread context

For each review thread, fetch three states of the commented file:

- **base** — content at `baseSha`
- **comment** — content at `originalCommitSha` (the state the reviewer saw)
- **head** — content at `headSha` (the state after the fix)

If the file was renamed between comment and head, resolve the new path (follow renames); use the
head-state path for the negative example. If any state cannot be fetched, skip the thread and
record the reason in the trace.

From these compute:

- `file_at_comment` — a slice of the comment-state file with 1-based line numbers, ±100 lines
  around the anchor. Problem regions are expressed in these line numbers.
- `diff base→comment` — what the PR changed before the review remark
- `diff comment→head` — how the remark was addressed (`-` = violation, `+` = fix)

### 3. Decide: extract or not

You extract static analysis rules from PR review threads. When the reviewer requests a change
that can be checked automatically, extract it.

**Extract when ALL three are true:**

1. Reviewer requested or suggested a concrete code change (includes rhetorical questions like
   "Why is X here?" — these are implicit change requests)
2. The pattern is GENERALIZABLE — it would apply in 10+ places across a large codebase
3. The check is STRUCTURALLY DETECTABLE — the violation can be found by analyzing code structure,
   types, names, API calls, or control flow

**Do NOT extract when the thread clearly falls into one of these categories:**

- No concrete change requested (pleasantries, vague suggestions like "could be cleaner")
- Requires understanding programmer intent or runtime information
- Truly one-off: specific to this exact bug/file with no recurring pattern
- Side observation not reflected in code: the comment→head diff does NOT show a corresponding
  code change

**Severity**: `ERROR` (bugs/security) | `WARNING` (smells) | `WEAK_WARNING` (style) | `INFO`
(suggestions).

If more context is needed to judge structural detectability (containing class/method, imports,
full change scope), fetch it with the available tools — but sparingly.

### 4. Compose the signal

For an extracted rule, fill the schema from [references/signal-schema.md](references/signal-schema.md):

- `ruleId`: unique kebab-case identifier; `ruleName`: short actionable title
- `description`: the problem, why it matters, and the fix
- `problemRegions` → line ranges in the **comment state** that tightly bound the violating
  expression — not the enclosing scope
- `positiveExamples`: the file at `originalCommitSha` with the problem ranges (violation present)
- `negativeExamples`: the file at `headSha` (violation fixed). Translate the problem ranges to the
  head state by locating the corresponding fixed code; if the code was deleted, use `null` ranges.
- `source`: full provenance — PR metadata, thread messages, both diffs, `verificationDate` =
  `closeTimestamp`

### 5. Report

After processing, print a short summary: PRs and threads processed, signals written (with file
names), threads skipped and why. Do not paste full signal JSONs into the summary.

## Quality bar

- One thread → at most one signal. Do not invent rules the reviewer didn't ask for.
- The `reason` field must state why the pattern is checkable by static analysis and cite the
  triggering comment.
- Prefer `NoRule` over a stretched generalization: a wrong signal costs more downstream than a
  missed one.
- Never fabricate SHAs, URLs, or line numbers — every value in the signal must come from the
  provider data or the fetched file contents.
