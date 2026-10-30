---
name: edict-promote-weak-signal
description: Managed worker that promotes weak signals by attempting fixes at up to five true-positive locations per generated inspection and opening a source-fix PR for user feedback.
---

# Promoting weak signal

Run only as a delegated managed subagent.
Follow [the managed task protocol](../edict_manager/references/task-protocol.md).
Registry ID: `edict-promote-weak-signal`.

Take the inspected project and your scratch directory from your run context. Call
`edict_prepare_weak_signal_promotion(token)`, passing `clusterIds` only when the assignment restricts the targets.
Selection considers Generated inspections with referenced strong positive Signals from fewer than five distinct source files and returns
up to five distinct, reviewed true-positive project locations per inspection. Clusters with an open source-fix review
or merged review awaiting signal publication are skipped. Missing persisted project review evidence requires a new generation/review run; report that prerequisite.

For each returned inspection:

1. Read its `inspectionDescription` and each selected location's exact source at `projectRevision`. Use read-only
   `git show <revision>:<path>` in the inspected project or `edict_file_at_ref` for unavailable objects. Retrieve the
   complete file before constructing its replacement. Check surrounding declarations and call sites as needed to
   understand the violation and preserve intended behavior.
2. Attempt a focused fix for each selected true positive. Make complete replacement files only in your own scratch
   directory, preserving unrelated code. Multiple locations in a file share one replacement. Do not edit the inspected
   checkout, inspection, examples, Signals, or cluster state. Do not expand the sample beyond the returned locations.
   Skip a location whose correction is ambiguous, unsupported, or cannot be made safely, and explain why. A TP
   classification identifies a violation; it does not establish that any particular fix is correct.
3. Review the proposed diff against the exact source revision and run relevant checks in scratch when feasible.
   Record the checks and results, including any validation that could not run. Exclude fixes that fail validation;
   do not submit an empty change or claim checks passed when they were unavailable.
4. If at least one location was fixed, call `edict_promote_weak_signal` with your token, `clusterId`, the returned
   `inspectionDigest`, `fixes`, and a concise `validation` report. Each fix contains `path`, `lines` (the selected
   locations' original starting lines), `content` (the complete replacement file), and `explanation`. Submit no more
   than five fixed locations in total for that inspection. The server creates one PR containing only these source
   files, describes the inspection and improvements, and asks users to accept/merge if they agree. It resolves the
   repository, reviewer, and target branch from configuration and keeps provider credentials private.

Publication is owned by the MCP server. Do not commit, push, or invoke a provider CLI yourself. If the target branch
has moved since analysis, report the stale evidence and request a new analysis instead of overwriting newer code.
An open review is reused. Identical fixes reuse their branch and PR on retry; changed fixes
after an interrupted publication use a different branch. A source-fix PR does not publish the inspection or
automatically turn weak evidence into a strong Signal.
The default pipeline checks the created PRs through the independent `edict-check-weak-signal-promotion` step; their
records under `promotions/active.json` keep only review references and states, grouped by source cluster. Covered merged
corrections become processed Signals in the originating cluster; other corrections go to the inbox for normal distribution.

Continue independent candidates after a per-inspection problem. Report every created/reused PR URL, fixed location,
skipped location and reason, validation limitation, and selection/publication failure. If no supported fix can be made,
report the no-op without opening a PR. Finish the managed task as failed when required evidence or publication failed;
otherwise complete it, including an explained no-op.
