---
name: edict-prepare
description: Validate inputs and capture a bounded read-only inbox snapshot for a managed Edict run.
---

# Edict Preparation

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-prepare`. This leaf has
no state-write operations.

Start the task. Verify the source project and private scratch location, and read `edict_registry` to confirm the
required managed call graph exists. Through `edict_list` and `edict_read`, select at most 100 inbox signal JSON files
alphabetically, unless the parent supplies a narrower set. Return the exact selected paths and hashes. Read every
selected record and reject missing IDs, invalid labels, absent source revisions, and malformed one-based ranges. Do not
repair malformed records here.
`SubmittedFeedback` records are supplied labelled source evidence. Accept both supported formats:
- `source.message` and `source.url`, with `idempotencyKey` and `provenance`;
- checked-in feedback with `source.inspectionName`, `inspectionDescription`, `codeSnippet`, `reason`, and
  `suggestionId`. These retain their supplied 12-digit signal IDs and need no `idempotencyKey` or `provenance` object.
Require the original feedback, source reference, exact file revision and ranges. Preserve the supplied format and
fields; do not normalize or rewrite records. They have no correcting diff; do not invent commit or PR provenance.

Distinguish malformed range structure from a documented source-location limitation in `SubmittedFeedback`.
If the feedback explicitly records a stale or overlong range and its supplied snippet is verifiable at the cited
revision, preserve the original record and return that limitation with the prepared snapshot for downstream semantic
processing. Do not reject the entire snapshot solely for that documented mismatch. Never invent missing lines or
claim the full range was verified. Missing source revisions, malformed range structure, or unverifiable evidence
remain prerequisite failures.


List existing cluster descriptions, all cluster member paths, and Pending cluster IDs for downstream navigation. The
distribution worker must read full candidate cluster membership before assignment. Embeddings and a worktree are not
prerequisites for this simpler pipeline. Require inspection capabilities only when generation has actual targets; an
empty run can complete without an inspection server.

Keep summaries in the task result or private scratch. Finish with the prepared snapshot and any prerequisite failure.
