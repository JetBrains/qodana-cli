---
name: managed-edict-next-batch-signal-analysis
description: Coordinate delegated analysis of a bounded Git commit selection and persist supported source-correction signals through edict-mcp.
---

# Managed Batch Signal Analysis

Follow [the managed protocol](../edict_manager/references/protocol.md)
and [the signal contract](../edict_manager/references/signals.md). Registry ID: `edict-next-batch-signal-analysis`. You
own coverage and `inbox.write`; workers own evidence inspection. Do not generate rules, clusters, examples, or
inspections.

Start the task. Require the source checkout and a bounded Git revision expression with a commit limit.
PR-review extraction belongs to `edict-next-pr-signal-analysis`; report misrouted review requests to the manager.
Do not require an IntelliJ
analysis session for local Git extraction. Keep temporary packages outside the state root.

1. Enumerate the supplied range in stable order with read-only Git, honoring its limit. Resolve each full
   revision, parent count, first parent, complete message, changed paths, and canonical unified diff. Exclude root
   commits, merges, reverts, automated commits, and commits without relevant source changes. Assign
   `commit-<first 16 revision characters>` IDs. A terse message alone is not grounds to exclude an otherwise eligible
   correction before source inspection.
2. For every eligible commit, including a singleton, create and delegate an `edict-next-signal-analysis` task with
   `operations: []`. Put that item's work-item ID, full commit/parent revisions and complete message
   in the `edict_delegate` prompt, together with its retained package, source checkout, revision
   readers if needed, and private scratch path. Pass only the returned short launch prompt to a fresh native subagent; it fetches
   the full assignment from `edict_task_get`. No inline fallback. Use waves within available concurrency.
3. Verify complete inspection coverage: returned inspected IDs must equal the prepared set exactly, with no duplicates,
   missing or blocked items. Every worker must have inspected complete human material plus exact source and diff. Stop
   on incomplete coverage. Preserve every supported worker finding; do not silently drop it for being cosmetic or local.
4. Materialize every candidate record in memory using the signal contract before any write. Check actual
   revision/path/ranges, changed-line intersections, label, canonical diff, complete source metadata, distinct stable
   ID, and absence of rule fields. Parse the candidate JSON and check that every `fileRevision.expectedRanges` item
   has integer `start` and `end` fields satisfying `1 <= start <= end`; `startLine`/`endLine` are unsupported.
   The number of materialized records must equal the number of accepted findings. On
   malformed evidence fail the batch without publishing a partial candidate set.
5. Publish each complete `FromCommit` record using `edict_state_write` with the supplied capability. If its stable inbox ID already
   exists with identical content, count it as an idempotent success. Conflicting content requires investigation and a
   failed outcome, not an overwrite. Read back all resulting paths and verify hashes/content and the complete expected
   ID set. A failed write may leave earlier valid records; report those exact paths for a retry.
6. Finish with inspected work-item IDs, signal IDs, paths, hashes, and counts. An empty eligible set or completely
   inspected batch with zero findings is a successful no-op with no placeholder records. No commit, push, or IntelliJ
   state mutation is part of publication.

The server checks signal structure and consistency with its supplied diff before writing. Evidence workers and this
coordinator must still verify source authenticity, canonical diff content, and semantic relevance.
