---
name: edict-retrospective-signal-analysis
description: Managed coordinator subagent that compares a stored inspection at an older Git snapshot and HEAD to extract verified fixes from findings that disappeared.
---

# Retrospective Signal Analysis

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md)
and [the signal contract](../edict_manager/references/signals.md). Registry ID:
`edict-retrospective-signal-analysis`. You own comparison coverage and Signal publication; delegated
`edict-signal-analysis` workers own semantic fix inspection. Do not change source, clusters, examples, or inspections.

Start the task. Require a source checkout, an Edict state directory, one cluster ID, and a Qodana runner tool that can
run a supplied inspection against an explicit project/revision worktree, write SARIF to private scratch, use a baseline,
and include absent results. Accept an optional end date and lookback period, defaulting to the current local date and
three calendar months. Stop before repository mutation if the runner contract is unavailable.

Resolve only these inputs from the Edict state:

```text
clusters/<cluster-id>/description.json
inspections/<cluster-id>.inspection.kts
```

Require both files and matching IDs. Treat the cluster as semantic authority and the inspection as its detector. The
state and caller's source checkout are read-only; put reports and all temporary files in private scratch outside both.
Use separate temporary Git worktrees for the historical revision and HEAD—never detach, stash, clean, reset, or
otherwise change the caller's checkout.

1. Resolve full `HEAD`. Subtract the requested period from the end date and select the newest first-parent commit at or
   before the cutoff that is reachable from `HEAD`. Stop if none exists or it equals `HEAD`. Create private detached
   worktrees at that exact revision and at HEAD. Install the captured inspection bytes only inside those temporary
   worktrees, recording any replaced bytes. Always remove both worktrees on exit; retain SARIF reports for verification.
2. Run the whole historical worktree project without a baseline. Require successful parseable SARIF containing results
   for the exact inspection ID. Run the whole HEAD worktree project with the same inspection bytes, historical SARIF as baseline, and
   absent-results mode enabled. Require another successful parseable report. Collect only exact-inspection results whose
   `baselineState` is `absent`; ignore new, unchanged, and unrelated rows. Absence is a candidate, not proof of intent.
3. For each absent result, use its historical path, range, message, and fingerprints to trace the file from the snapshot
   to HEAD, following renames. Identify the first single-parent commit that removes that concrete finding. Retain its
   full revision and parent, complete message, canonical unified diff, and relevant source. Exclude merges, reverts,
   automated/generated churn, incidental deletion or movement, and ambiguous broad refactors. Deduplicate rows removed
   by the same before/after construct.
4. Create one managed `edict-signal-analysis` child per retained fixing commit. Include the cluster description,
   inspection metadata, absent rows, stable `commit-<first 16 revision characters>` work-item ID, exact commit package,
   source checkout, and private scratch path. Each runs in a fresh native subagent; never inspect it inline. Require
   complete worker coverage and retry lost workers when possible. A fix qualifies only when the parent still contains
   the reported violation and the commit deliberately corrects the same semantic problem detected by the stored
   inspection.
5. Materialize supported findings using the current signal contract. `POSITIVE` uses the fixing commit's parent and a
   minimal range intersecting removed lines; `NEGATIVE` uses the fixing commit and a minimal range intersecting added
   lines. Intentional deletion with no corrected range produces only a positive Signal. Use `FromCommit` with only the
   full fixing `commitRevision` and optional URL; the server derives and validates its parent, message, and canonical
   diff from the commit object. Use a stable commit work-item provenance ID, deterministic idempotency keys, and
   `syntheticExampleId: null`. Do not add null ranges, cluster IDs, or retrospective IDs to unsupported schema fields.
6. Parse and validate the complete candidate set before any write. Verify each revision/path/range against Git and the
   canonical diff, then publish through `edict_publish_signal`. An identical existing model is an idempotent success;
   never overwrite different content. Read back and verify every returned Signal model.
7. Finish with cutoff and snapshot revisions, original HEAD, both SARIF paths, exact-inspection and absent counts,
   inspected work-item IDs, accepted fixing commits, rejected candidates with reasons, and created or existing Signal
   paths and hashes. A successful complete comparison with no intentional fixes is a no-op.
