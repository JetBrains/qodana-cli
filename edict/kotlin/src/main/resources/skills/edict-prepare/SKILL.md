---
name: edict-prepare
description: Validate inputs and capture a bounded read-only inbox snapshot for a managed Edict run.
---

# Edict Preparation

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-prepare`. This leaf has
no state-write operations.

Start the task. Verify the source project and private scratch location, and read `edict_registry` to confirm the
required managed call graph exists. Through `edict_list`, select at most 100 inbox signal JSON files alphabetically,
unless the parent supplies a narrower set. Call `edict_validate_inbox` once with the complete selected path list and
retain its full receipt. Read every selected record through `edict_read`, require its returned hash to match the receipt,
and reject missing IDs, invalid labels, absent source revisions, and malformed one-based ranges. Do not repair malformed
records here.
`SubmittedFeedback` records are supplied labelled source evidence. Accept both supported formats:
- `source.message` and `source.url`, with `idempotencyKey` and `provenance`;
- checked-in feedback with `source.inspectionName`, `inspectionDescription`, `codeSnippet`, `reason`, and
  `suggestionId`. These retain their supplied 12-digit signal IDs and need no `idempotencyKey` or `provenance` object.
Require the original feedback, source reference, exact file revision and ranges. Preserve the supplied format and
fields; do not normalize or rewrite records. They have no correcting diff; do not invent commit or PR provenance.

Validate source evidence against `fileRevision.revision`, never against the current checkout merely because the path
exists there. If the cited revision is not the checked-out `HEAD` and no exact-revision inspection tool is available,
use read-only Git from the supplied source project (for example,
`git show <revision>:<path>`) and count/match ranges in those bytes. A range beyond the current worktree file is not
invalid when it exists at the cited revision. Treat failure to resolve the cited Git object as unverifiable evidence;
do not silently fall back to `HEAD`.

Distinguish malformed range structure from a source-location limitation in `SubmittedFeedback`. Imported inspection
feedback may retain stale inspection coordinates after the source fixture is minimized or rebased. When the cited
range is overlong or points to different lines but the supplied `codeSnippet` is found verbatim at the cited revision,
preserve the original record and return the cited range, verified snippet location, and mismatch as a limitation for
downstream semantic processing. This fallback does not require the feedback prose itself to say that the range is
stale. Do not reject the snapshot solely for this mismatch, rewrite the range, invent missing lines, or claim the cited
range was verified. Missing source revisions, malformed range structure, or evidence for which neither the cited
range nor supplied snippet can be verified remain prerequisite failures.


List existing cluster descriptions, all cluster member paths, and Pending cluster IDs for downstream navigation. The
distribution worker must read full candidate cluster membership before assignment. Preparation validates the registered
state in place; the host already supplied the checkout. The distribution task lazily prepares same-language embedding
neighbours from this frozen receipt and reuses the durable cache under the managed state root. Worktree creation and
legacy IDE session preparation are not managed prerequisites. Require inspection capabilities only when generation has actual targets; an
empty run can complete without an inspection server.

Return the existing state location, source project, complete validation receipt, existing-cluster navigation, and any
prerequisite failure. The coordinator must pass the receipt unchanged to post-distribution validation. Do not distribute
Signals, create examples, or generate inspections in this stage. Keep summaries in the task result or private scratch
and finish the task.
