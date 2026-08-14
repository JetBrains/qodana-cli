---
name: qodana-batch-signal-analysis
description: Analyze selected pull-request discussions or corrective commits for human-confirmed strong signals and publish validated signal records to a local rules-repository inbox. Use only when explicitly invoked with a Qodana analysis session ID and the Qodana analysis MCP tools.
---

# Qodana Batch Signal Analysis

Analyze every selected PR discussion or eligible corrective commit and extract strong signals directly. Do not formulate or persist rules, group signals, search unrelated history, generate examples, or implement inspections. A later clustering stage creates rules.

MCP owns PR-provider access and validates submitted packages and findings. Use local Git for source inspection, commit discovery, and rules-repository publication.

## Inputs

Require:

- A Qodana analysis session ID.
- Exactly one source mode:
   - explicit PR numbers or an inclusive date range; or
   - one bounded Git revision expression.
- PR or commit limits.
- The analyzed source project path for Qodana MCP routing.
- A local rules-repository checkout when signals must be persisted.

Stop with a clear error when an input or required MCP tool is unavailable.

## Workflow

1. Prepare one source mode.
   - PR mode: call `mcp__qodana__prepare_pr_analysis`, then call `mcp__qodana__list_pr_analysis_items` starting at offset 0. Continue with every returned `nextOffset` until it is absent. For each page, require the item count to equal the smaller of the requested limit and the number of remaining items. Preserve the ordered union of page items. Require its size to equal `totalWorkItemCount` and require every work-item ID to be distinct before inspection.
   - Commit mode: enumerate the bounded range with read-only `git log`, preserving order and the configured limit. Resolve each commit revision, first parent, complete message, and unified diff with `git show` and `git diff`. Exclude roots, merges, reverts, automated commits, and commits without relevant source changes. Retain eligible packages locally and assign `commit-<first 16 revision characters>` work-item IDs.
2. Inspect every work item, using the parallel inspection protocol below when applicable. Treat an empty PR batch or eligible commit list as a successful no-op.
   - PR mode: call `mcp__qodana__get_pr_analysis_item`, read the complete human discussion, and inspect base, comment, and head source with local Git.
   - Commit mode: inspect the complete retained package and compare its parent and commit sides. MCP never stores or returns commit packages.
3. Verify complete inspection coverage against the paginated prepared set, then collect only accepted strong signals. Coverage bookkeeping is internal; do not create a signal, persisted result, or inbox placeholder for work items without signals.
4. Validate the signal array exactly once. An empty array is valid.
   - PR mode: call `mcp__qodana__validate_pr_signals` with the complete ordered `inspectedWorkItemIds` list and `signalsJson`. Validation must fail when coverage is incomplete.
   - Commit mode: call `mcp__qodana__validate_commit_signals` with the original range and limit plus ordered `itemsJson` and `signalsJson`. This is the only time commit packages are sent to MCP.
5. If the receipt reports zero signals, finish successfully without writing files.
6. For every validated signal, write one `inbox/<signal-id>.json` record using the storage contract below.
7. Call `mcp__qodana__validate_inbox_changes` once with exactly the new paths. Recompute every returned SHA-256.
8. Stop without staging when validation fails or unrelated staged changes exist. Otherwise stage only validated paths and commit once. Never amend or push.
9. Report receipt IDs, signal IDs, and the commit SHA.

For every Qodana MCP call, route `projectPath` to the analyzed source project. Never use the rules-repository path or temporary Codex workspace as `projectPath`.

## Parallel inspection and coverage

When there are two or more work items and subagent tools are available, use subagents. The coordinator must retain ownership of orchestration and finalization while workers perform bounded, independent inspection tasks.

1. Exhaust every work-item page and record the complete ordered set of prepared work-item IDs before inspection. Never infer completeness from the number of items visible in one MCP response.
2. Partition that set into disjoint, non-empty chunks. Use no more workers concurrently than the runtime permits, and assign every work item to exactly one worker. Do not create one worker per item when bounded chunks will use the available concurrency more efficiently.
3. Give each worker only inspection work. For every assigned item, the worker must inspect the complete human material and the relevant source before and after the correction:
   - PR mode: the complete discussion, PR context, anchored source at the applicable revisions, and the relevant before-to-after diff.
   - Commit mode: the complete retained commit message and the parent-to-commit source diff.
4. Do not pre-filter or reject an item from discussion text, commit message, title, or metadata alone. Terse human material still requires source and diff inspection.
5. Require each worker to return exactly one internal outcome for every assigned work-item ID:
   - `SIGNALS`: one or more candidate signal objects satisfying the signal contract, with a concise evidence-based rationale.
   - `NO_SIGNAL`: no candidate signals, with a concise reason grounded in the inspected human material and source change.
   - `BLOCKED`: inspection could not be completed, identifying the missing material or failed operation.
6. Workers must not call either signal-validation tool, write inbox files, modify the rules repository, stage changes, or commit.
7. The coordinator must maintain an internal coverage ledger and verify that returned work-item IDs exactly equal the prepared set, with no omissions or duplicates. It must also verify that every outcome reflects completed human-material and source-diff inspection.
8. Retry missing or blocked inspection when possible. If any work item remains missing, duplicated, blocked, or incompletely inspected, stop with a clear error before signal validation. Never interpret incomplete coverage as `NO_SIGNAL`.
9. Only after complete coverage may the coordinator combine candidate signals, apply the acceptance criteria, and perform the single validation call required by the workflow.

If subagent tools are unavailable, follow the same coverage protocol sequentially in the coordinator. For a single work item, inspect it directly without delegation.

## Signal acceptance

A strong signal must be grounded in human material:

- a human review discussion that requests, identifies, or confirms a correction when interpreted together with its PR context and anchored code change; or
- a human-authored corrective commit whose message and diff clearly demonstrate the concern.

For PRs, evaluate the complete discussion, PR title and body, anchored source, and relevant before-to-after diff together. A discussion does not need to restate the full concern when it clearly identifies or affirms the anchored change as a correction and the combined human-authored context makes the concern unambiguous. Do not reject a finding solely because the discussion is terse. A signal captures a confirmed code concern, so do not require the human material to formulate a reusable rule.

Accept only findings with concrete source evidence and enough combined context to describe what the human confirmed without relying on information outside the supplied material. Reject generated-code churn, merges, reverts, cosmetic edits, feature additions, one-off identifier changes, speculative interpretations, and concerns requiring unavailable project intent or runtime information.

Do not produce a rule name, statement, language, severity, rule ID, or proposed implementation.

### Labels

- `POSITIVE`: the problematic form confirmed by the human signal.
- `NEGATIVE`: the corrected or compliant form confirmed by the human signal.

For commits, `POSITIVE` must use the parent revision and `NEGATIVE` the correcting commit. For PRs, use only paths and revisions belonging to the returned work item.

## Signal contract

Submit a JSON array containing only discovered signals:

```json
[
  {
    "workItemId": "stable work-item ID",
    "sourceType": "PR_DISCUSSION | COMMIT",
    "label": "POSITIVE | NEGATIVE",
    "evidence": {
      "path": "src/Example.kt",
      "revision": "<validated revision>",
      "ranges": [{"start": 1, "end": 1}],
      "description": "what the human-confirmed source range demonstrates"
    }
  }
]
```

Use `[]` when no strong signals are found. Do not invent work-item IDs, paths, revisions, or ranges.

## Inbox storage

Convert each validated finding directly into one signal record:

```json
{
  "id": "s-<10 lowercase hex>",
  "idempotencyKey": "<stable source provenance key>",
  "fileRevision": {
    "path": "<finding evidence path>",
    "revision": "<finding evidence revision>",
    "expectedRanges": [{"start": 1, "end": 1}]
  },
  "source": {"type": "FromPR or FromCommit"},
  "label": "POSITIVE | NEGATIVE",
  "description": "<finding evidence description>",
  "syntheticExampleId": null,
  "provenance": {
    "workItemId": "<work-item ID>",
    "analysisBatchId": "<validated batch ID>"
  }
}
```

Construct the idempotency key from immutable source provenance, work-item ID, signal index, label, path, revision, and ranges. Set `id` to `s-` plus the first 10 lowercase hexadecimal characters of its SHA-256.

For `FromPR`, preserve PR number, title, complete discussion messages, relevant before-to-after diff, and discussion URL. For `FromCommit`, preserve correcting revision, first parent, complete message, validated corrective diff, and commit URL when available.

Do not add a rule, rule ID, or proposed statement to a signal. Keep orchestration identifiers in `provenance`, leave `syntheticExampleId` null, and treat successful MCP receipts plus the local commit as the durable outcome.
