---
name: edict-batch-signal-analysis
description: Coordinate delegated analysis of a bounded Git commit selection and persist supported source-correction signals through edict-mcp.
---

# Batch Signal Analysis

Follow [the managed protocol](../edict_manager/references/protocol.md)
and [the signal contract](../edict_manager/references/signals.md). Registry ID: `edict-batch-signal-analysis`. You
own coverage and `inbox.write`; workers own evidence inspection. Do not generate rules, clusters, examples, or
inspections.

Start the task. Require the source checkout and a bounded Git revision expression with a commit limit.
PR-review extraction belongs to `edict-pr-signal-analysis`; report misrouted review requests to the manager.
Do not require an IntelliJ
analysis session for local Git extraction. Keep temporary packages outside the state root.

1. Enumerate the supplied range in stable order with read-only Git, honoring its limit. Resolve each full
   revision, parent count, first parent, complete message, changed paths, and canonical unified diff. Exclude root
   commits, merges, reverts, automated commits, and commits without relevant source changes. Assign
   `commit-<first 16 revision characters>` IDs. A terse message alone is not grounds to exclude an otherwise eligible
   correction before source inspection.
2. Partition the prepared set into disjoint, non-empty chunks of at most eight work items, preserving their order.
   Create and delegate one `edict-signal-analysis` task per chunk with `operations: []`. Include every assigned
   work-item ID, full commit/parent revisions and complete message
   in the `edict_delegate` prompt, together with its retained packages, source checkout, revision
   readers if needed, and private scratch path. Pass only the returned short launch prompt to a fresh native subagent; it fetches
   the full assignment from `edict_task_get`. Even a singleton runs in a managed child; there is no inline fallback.
   Use waves within available concurrency, never larger chunks just to fit one wave. Assign each item exactly once.
3. Verify complete inspection coverage: returned inspected IDs must equal the prepared set exactly, with no duplicates,
   missing or blocked items. Every worker must have inspected complete human material plus exact source and diff. Stop
   on incomplete coverage after retrying lost or blocked inspection when possible. Preserve every supported worker
   finding; do not silently drop or downgrade it for being cosmetic, local, or insufficiently generalizable.
4. Materialize every candidate record in memory using the signal contract before any write. Check actual
   revision/path/ranges, changed-line intersections, label, canonical diff, complete source metadata, distinct stable
   ID, and absence of rule fields. Parse the candidate JSON and check that every `fileRevision.expectedRanges` item
   has integer `start` and `end` fields satisfying `1 <= start <= end`; `startLine`/`endLine` are unsupported.
   The number of materialized records must equal the number of accepted findings. On
   malformed evidence fail the batch without publishing a partial candidate set.
   Build `source.message` and `source.diffPositiveToNegative` by copying the retained Git output programmatically,
   not by retyping the worker's prose. Preserve the original message punctuation and internal newlines; adding a
   sentence-ending period changes the evidence. Only terminal newlines introduced by Git's message formatter may
   be omitted. Check equality with the retained source values before publication.
   Compute the stable signal ID with a SHA-256 implementation over the exact UTF-8 idempotency key; never supply a
   guessed digest. Preserve the diff's final newline when serializing it into the candidate JSON.
5. Publish each complete `FromCommit` record using `edict_state_write` with the supplied capability. If its stable inbox ID already
   exists with identical content, count it as an idempotent success. Conflicting content requires investigation and a
   failed outcome, not an overwrite. Read back all resulting paths and verify hashes/content and the complete expected
   ID set. A failed write may leave earlier valid records; report those exact paths for a retry.
6. Finish with inspected work-item IDs, signal IDs, paths, hashes, and counts. An empty eligible set or completely
   inspected batch with zero findings is a successful no-op with no placeholder records. No commit, push, or IntelliJ
   state mutation is part of publication.

The server checks signal structure and consistency with its supplied diff before writing. Evidence workers and this
coordinator must still verify source authenticity, canonical diff content, and semantic relevance.
