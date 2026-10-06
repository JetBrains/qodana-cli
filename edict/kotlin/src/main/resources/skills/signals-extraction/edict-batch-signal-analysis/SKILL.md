---
name: edict-batch-signal-analysis
description: Managed coordinator subagent that analyzes bounded Git commits and persists supported source-correction Signals.
---

# Batch Signal Analysis

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md)
and [the signal contract](../edict_manager/references/signals.md). Registry ID: `edict-batch-signal-analysis`. You
own coverage and Signal publication; workers own evidence inspection. Do not generate rules, clusters, examples, or
inspections.

Start the task. Take the source checkout (`projectDirectory`) and private scratch root (`scratchDirectory`) from `edict_context`. Require a
bounded Git revision expression with a commit limit.
PR-review extraction belongs to `edict-pr-signal-analysis`; report misrouted review requests to the manager.

1. Enumerate the supplied range in stable order with read-only Git, honoring its limit. Resolve each full
   revision, parent count, first parent, complete message, changed paths, and canonical unified diff. Exclude root
   commits, merges, reverts, automated commits, and commits without relevant source changes. Assign
   `commit-<first 16 revision characters>` IDs. A terse message alone is not grounds to exclude an otherwise eligible
   correction before source inspection.
2. Create and delegate one `edict-signal-analysis` task per prepared work item, preserving
   stable order. Every task owns exactly one commit; never combine several commits in one evidence worker. Include its
   work-item ID, full commit/parent revisions and complete message
   in the `edict_delegate` prompt, together with its retained packages, source checkout, revision
   readers if needed, and private scratch path. Pass only the returned short launch prompt to a fresh native subagent; it fetches
   the full assignment from `edict_task_get`. Even a singleton runs in a managed child; there is no inline fallback.
   Use waves within available concurrency. Assign each item exactly once.
3. Verify complete inspection coverage: returned inspected IDs must equal the prepared set exactly, with no duplicates,
   missing or blocked items. Every worker must have inspected complete human material plus exact source and diff. Stop
   on incomplete coverage after retrying lost or blocked inspection when possible. Preserve every supported worker
   finding; do not silently drop or downgrade it for being cosmetic, local, or insufficiently generalizable.
4. Materialize every candidate model in memory using the signal contract before publication. Check actual
   revision/path/ranges, changed-line intersections, label, canonical diff, complete source metadata, distinct stable
   ID, and absence of rule fields. Parse the candidate JSON and check that every `fileRevision.expectedRanges` item
   has integer `start` and `end` fields satisfying `1 <= start <= end`; `startLine`/`endLine` are unsupported.
   The number of materialized records must equal the number of accepted findings. On
   malformed evidence fail the batch without publishing a partial candidate set.
   For each `FromCommit` source, persist only the full correcting `commitRevision` and an external `url` when one is
   available. Do not copy its parent, message, or diff into the Signal; the server derives them from the commit object.
   Compute the stable signal ID with a SHA-256 implementation over the exact UTF-8 idempotency key; never supply a
   guessed digest.
5. Publish each complete `FromCommit` model using `edict_publish_signal` with the supplied capability. If its stable ID
   already exists with the same model, count it as an idempotent success. A conflicting model requires investigation and
   a failed outcome, not an overwrite. Verify the complete expected ID set. A failed call may leave earlier valid models;
   report those exact IDs for a retry.
6. Finish with inspected work-item IDs, signal IDs, and counts. An empty eligible set or completely
   inspected batch with zero findings is a successful no-op with no placeholder records. No commit, push, or IntelliJ
   state mutation is part of publication.

The server resolves each commit and checks the selected evidence side, path, and ranges against the canonical repository
diff before writing. Evidence workers and this coordinator must still verify semantic relevance.
