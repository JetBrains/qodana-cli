---
name: qodana-batch-signal-analysis
description: Analyze selected pull-request discussions or corrective commits for human-confirmed strong signals and publish validated signal records to a local rules-repository inbox. Use only when explicitly invoked with a Qodana analysis session ID and the Qodana analysis MCP tools.
---

# Qodana Batch Signal Analysis

Analyze every selected PR discussion or eligible corrective commit and extract strong signals directly. Optimize this stage for recall of human-confirmed source corrections. Do not formulate or persist rules, group signals, search unrelated history, generate examples, or implement inspections. A later clustering stage decides whether evidence is reusable and creates rules.

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
2. Partition that set into disjoint, non-empty chunks of at most 8 work items. Keep discussions from one PR together when this does not exceed the limit. Give workers explicit work-item IDs rather than page offsets or inferred ranges. Run additional worker waves until every chunk is complete; never enlarge chunks merely to fit all work into one wave. Use no more workers concurrently than the runtime permits, and assign every work item to exactly one worker.
3. Give each worker only inspection work. For every assigned item, the worker must inspect the complete human material and the relevant source before and after the correction:
    - PR mode: the complete discussion, PR context, anchored source at the applicable revisions, and the relevant before-to-after diff.
    - Commit mode: the complete retained commit message and the parent-to-commit source diff.
    - When a PR revision is absent from the local checkout, use the available Qodana Git-backed revision file and diff readers. Mark the item `BLOCKED` only after both local Git and those revision readers fail; never substitute current-checkout source or treat a missing local object as `NO_SIGNAL`.
4. Do not pre-filter or reject an item from discussion text, commit message, title, or metadata alone. Terse human material still requires source and diff inspection.
5. Require each worker to apply the signal-acceptance checklist independently to every item and return exactly one internal outcome for every assigned work-item ID:
    - `SIGNALS`: every supported signal object satisfying the signal contract, with a concise evidence-based rationale. Do not return only the most severe or most general finding.
    - `NO_SIGNAL`: no candidate signals, with a concise reason that identifies which acceptance condition is absent and is grounded in the inspected human material and source change.
    - `BLOCKED`: inspection could not be completed, identifying the missing material or failed operation.
6. Workers must not call either signal-validation tool, write inbox files, modify the rules repository, stage changes, or commit.
7. The coordinator must maintain an internal coverage ledger and verify that returned work-item IDs exactly equal the prepared set, with no omissions or duplicates. It must also verify that every outcome reflects completed human-material and source-diff inspection.
8. Retry missing or blocked inspection when possible. If any work item remains missing, duplicated, blocked, or incompletely inspected, stop with a clear error before signal validation. Never interpret incomplete coverage as `NO_SIGNAL`.
9. Only after complete coverage may the coordinator combine candidate signals and perform the single validation call required by the workflow. The coordinator must not silently discard or downgrade a worker's `SIGNALS` outcome based on a stricter usefulness, severity, or generalizability judgment. If the coordinator believes a candidate lacks required provenance or contradicts the inspected source, reinspect that item and record the evidence-based decision before excluding it.

If subagent tools are unavailable, follow the same coverage protocol sequentially in the coordinator. For a single work item, inspect it directly without delegation.

## Signal acceptance

A PR discussion produces one or more strong signals when all of these conditions hold:

1. Human material requests, suggests, identifies, or confirms a concrete source change. Questions, rhetorical questions, short alternatives, and `nit` comments count when they clearly imply a change.
2. The anchored before-to-after change implements or demonstrates that request. A resolved marker alone is not evidence, but it may accompany evidence in the source diff.
3. The problematic or corrected form can be bounded to concrete source ranges and described without hidden information.

For commits, accept a human-authored corrective commit when its message and diff clearly demonstrate the same three conditions.

Evaluate the complete discussion, PR title and body, anchored source, and relevant before-to-after diff together. A discussion does not need to restate the full concern when it clearly identifies or affirms the anchored change as a correction. Do not reject a finding solely because the discussion is terse, polite, phrased as a question, marked as a nit, or supplies only the preferred replacement.

This stage collects evidence rather than deciding whether to create a rule. Do not require a finding to be severe, bug-like, repository-wide, independently generalizable, or likely to occur a particular number of times. Human-confirmed style, naming, imports, documentation, configuration, API choice, lifecycle, refactoring, and local correctness changes are eligible when the three conditions hold. Do not reject a supported correction as cosmetic or one-off; downstream clustering decides its reuse value.

Reject only items without a concrete requested or confirmed source correction, items whose relevant code did not change accordingly, generated-code churn, merges, reverts, speculative interpretations, and concerns that cannot be tied to source evidence without unavailable intent or runtime information.

Do not produce a rule name, statement, language, severity, rule ID, or proposed implementation.

### Labels

- `POSITIVE`: the problematic form confirmed by the human signal.
- `NEGATIVE`: the corrected or compliant form confirmed by the human signal.

When a qualifying correction replaces one source form with another, emit both the tightly bounded `POSITIVE` before evidence and `NEGATIVE` after evidence when both sides are available. For deletion-only or addition-only corrections, emit the supported side. Every qualifying work item must contribute at least one signal.

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

For `FromPR`, use this source shape:

```json
{
  "type": "FromPR",
  "prNumber": 123,
  "title": "<complete PR title>",
  "discussionMessages": ["<complete human message body>"],
  "diffPositiveToNegative": "<actual unified before-to-after diff>",
  "url": "<discussion URL>"
}
```

`diffPositiveToNegative` must contain the relevant unified diff with hunk and changed lines. Never put availability diagnostics, revision summaries, placeholders, or text such as `UNAVAILABLE` in this field. If the diff cannot be obtained, the item is `BLOCKED` and must not be persisted.

For `FromCommit`, preserve correcting revision, first parent, complete message, actual unified corrective diff, and commit URL when available.

Keep `description` to one concise line explaining only what the selected evidence range demonstrates for its label. Do not append discussion messages, serialized JSON, preserved context, diffs, availability status, revisions, provenance, or orchestration diagnostics. Those values belong in `source`, `fileRevision`, and `provenance`.

Do not add a rule, rule ID, or proposed statement to a signal. Keep orchestration identifiers in `provenance`, leave `syntheticExampleId` null, and treat successful MCP receipts plus the local commit as the durable outcome.
