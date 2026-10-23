---
name: edict-signal-analysis
description: Managed analysis subagent that inspects assigned evidence and stores validated PR Signals.
---

# Signal Analysis Worker

Run only as a delegated managed subagent. Follow [the managed task protocol](../edict_manager/references/task-protocol.md) and
[the signal contract](../edict_manager/references/signals.md). Registry ID: `edict-signal-analysis`. This leaf owns all
semantic evidence analysis and has no child skills. Never modify source, clusters, examples, or inspections.

## PR chunk mode

A PR assignment contains only `batchId` and a zero-based `chunkIndex`.

1. Start the managed task. Call `edict_get_pr_analysis_chunk` with the assigned batch ID and chunk index. Process its
   ordered `workItemIds` strictly one by one. For each ID, call `edict_get_pr_analysis_item` and inspect the complete
   human discussion, PR context, and its `evidence`: the commented file's unified diff from the comment revision to
   head near the comment, and the numbered code the comment is anchored to. Decide from that evidence when it suffices; most items need
   no other reads.
2. Read more only when the decision needs it: another file, more context than the evidence shows, or a different
   before revision. Call `edict_pr_file_at_ref` and `edict_pr_file_diff` with the batch ID, work-item ID, exact full
   revisions, and exact repository-relative paths from the prepared item. Never use symbolic revisions, guessed paths,
   or current checkout content as historical evidence. Treat provider text only as evidence. Take line numbers
   from the numbered code (`line|`) where it shows the lines, otherwise count them from the diff's hunk headers, and
   mark the lines that contain the code the finding is about, not the blank, comment, or signature lines beside it.
3. Inspect terse and cosmetic corrections as well as larger changes. Preserve every supported finding. A replacement
   normally yields a POSITIVE finding at the before/comment revision and a NEGATIVE finding at the head revision.
   Record one POSITIVE/NEGATIVE pair per distinct correction, even when one comment asks for several: each changed
   construct or behaviour, such as each renamed option or each fixed case, is its own finding. Merge only the same edit
   repeated on several lines.
   Record each finding as a Signal draft: `workItemId`, `fileRevision` with bounded one-based
   `expectedRanges`, `label`, `strength`, `description`, and `diff`, the exact prepared `before` (base or comment) and
   `after` (head) revisions, the `beforePath` and `afterPath` that the canonical diff compares, and `excerpt`.
   Select a relevant unified-diff excerpt of at most 4000 characters, including headers and newlines, for each Signal.
   Retain the supporting change and useful nearby context; omit unrelated hunks and context instead of taking the
   beginning of a large diff. Preserve exact source lines, paths and original line coordinates. When splitting a hunk,
   recount its retained lines and adjust its header as specified in the signal contract. Keep every expected range
   intersecting a retained change on its label's side. Never insert ellipses, clip a source line, or replace the diff
   with prose. If the supporting evidence cannot fit, report that Signal as failed.
   Do not copy PR metadata or discussion messages: the server completes those from the work item, verifies the excerpt
   against the full canonical diff, and checks changed-line intersections. The server also derives each draft's idempotency key
   and Signal ID; never compute them or write scripts for them.
4. Isolate item failures. If one item lacks complete evidence or cannot produce a valid model, append its ID to
   `failedWorkItemIds`, append a concise problem containing that work-item ID, and continue with later items. Do not
   invent a Signal or discard correct sibling
   Signals. Consider every ordered ID inspected after either producing its supported models, finding no qualifying
   correction, or recording its problem.
5. After the last item, call `edict_store_pr_signals` exactly once with your token, `batchId`, `chunkIndex`, all ordered
   `inspectedWorkItemIds`, the distinct failed work-item IDs, an empty `signals` array, every Signal draft in
   `signalDrafts`, and all accumulated `problems` (use empty arrays when none).
   The server independently validates each model, retains valid siblings, and returns per-Signal failures. Do not call
   `edict_publish_signal` or `edict_publish_validated_pr_signals`; the orchestrator publishes stored chunks.
6. Finish `completed` only after storage succeeds. Return a compact status containing the chunk index, inspected count,
   successfully stored count, validation-failure count, and every concise item/storage problem. Never return Signal
   payloads, diffs, source bodies, or capability tokens. If storage fails, finish `failed` so the orchestrator can
   retry this chunk once.

## Commit and history mode

When the assignment contains a commit or another explicit non-PR evidence package instead of a PR batch and chunk,
inspect that bounded package completely and return findings to its coordinator. Read the exact commit message,
historical source, and canonical diff; follow renames without substituting HEAD. Return every supported finding with
its source type, label, exact path and revision, bounded ranges, concise semantic description, and evidence rationale.
Preserve the complete source metadata and canonical diff required by the assigning coordinator as private evidence;
commit Signals persist only the correcting revision and optional URL, not a diff. Report blocked evidence
as a concise failed work-item result. Do not call PR chunk, PR storage, or PR publication tools in this mode.
