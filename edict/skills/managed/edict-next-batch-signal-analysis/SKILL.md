---
name: managed-edict-next-batch-signal-analysis
description: Coordinate delegated review of a bounded commit or PR selection and persist supported source-correction signals through edict-mcp.
---

# Managed Batch Signal Analysis

Follow [the managed protocol](../edict_manager/references/protocol.md)
and [the signal contract](../edict_manager/references/signals.md). Registry ID: `edict-next-batch-signal-analysis`. You
own coverage and `inbox.write`; workers own evidence inspection. Do not generate rules, clusters, examples, or
inspections.

Start the task. Require the source checkout and exactly one bounded source selection: a Git revision expression with a
commit limit, or explicit PR numbers/date bounds with a PR limit and a read-only provider. Do not require an IntelliJ
analysis session for local Git extraction. Keep temporary packages outside the state root.

1. Commit mode: enumerate the supplied range in stable order with read-only Git, honoring its limit. Resolve each full
   revision, parent count, first parent, complete message, changed paths, and canonical unified diff. Exclude root
   commits, merges, reverts, automated commits, and commits without relevant source changes. Assign
   `commit-<first 16 revision characters>` IDs. A terse message alone is not grounds to exclude an otherwise eligible
   correction before source inspection.
2. PR mode: read the bounded selection and every discussion through the available provider's read-only interface. Follow
   all pagination, preserve ordered distinct work-item IDs, and require the final count to equal the provider's total
   when supplied. Retain full human discussion, PR title/body, URL, and exact before/after revision anchors. Do not use
   a legacy IntelliJ preparation tool that changes Edict state.
3. For every eligible work item, including a singleton, create and delegate an `edict-next-signal-analysis` task with
   `operations: []`. In `edict_task_add`, put the work-item ID, full commit revision and commit subject in the title
   (for PRs, use the PR number, URL and title). This records the source assignment in the plan and logs before the
   worker starts; a generic title such as "Review correction" is insufficient. Give a fresh native subagent exactly
   that same item's retained package, source checkout, source revision
   readers if needed, and private scratch path. No inline fallback. Use waves within available concurrency.
4. Verify complete inspection coverage: returned inspected IDs must equal the prepared set exactly, with no duplicates,
   missing or blocked items. Every worker must have inspected complete human material plus exact source and diff. Stop
   on incomplete coverage. Preserve every supported worker finding; do not silently drop it for being cosmetic or local.
5. Materialize every candidate record in memory using the signal contract before any write. Check actual
   revision/path/ranges, changed-line intersections, label, canonical diff, complete source metadata, distinct stable
   ID, and absence of rule fields. Parse the candidate JSON and check that every `fileRevision.expectedRanges` item
   has integer `start` and `end` fields satisfying `1 <= start <= end`; `startLine`/`endLine` are unsupported.
   The number of materialized records must equal the number of accepted findings. On
   malformed evidence fail the batch without publishing a partial candidate set.
6. Publish each complete record using `edict_state_write` with the supplied capability. If its stable inbox ID already
   exists with identical content, count it as an idempotent success. Conflicting content requires investigation and a
   failed outcome, not an overwrite. Read back all resulting paths and verify hashes/content and the complete expected
   ID set. A failed write may leave earlier valid records; report those exact paths for a retry.
7. Finish with inspected work-item IDs, signal IDs, paths, hashes, and counts. An empty eligible set or completely
   inspected batch with zero findings is a successful no-op with no placeholder records. No commit, push, or IntelliJ
   state mutation is part of publication.

The server checks signal structure and consistency with its supplied diff before writing. Evidence workers and this
coordinator must still verify source authenticity, canonical diff content, and semantic relevance.
