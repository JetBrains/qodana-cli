# Strong-signal evidence and storage

## Existing submitted feedback

The inbox can already contain `SubmittedFeedback` records supplied with the project. Process these in place, preserving
their original fields and IDs. The checked-in format uses a 12-digit hexadecimal signal ID and source fields
`inspectionName`, `inspectionDescription`, `codeSnippet`, `reason`, and `suggestionId`; `suggestionId` is its stable
source reference. This format has no `idempotencyKey`, `provenance`, or correcting diff. The alternative feedback format
uses `source.message`, `source.url`, `idempotencyKey`, and `provenance`. Both carry an exact `fileRevision`, label, and
description. Do not invent missing commit/PR metadata or convert existing feedback into extracted commit signals.

## Extracting commit and PR signals

Accept a PR discussion or human-authored corrective commit when human material requests, identifies, or confirms a
concrete source correction, the exact before/after change demonstrates it, and its problematic or corrected form is
bounded to source ranges without hidden information. Inspect complete discussion, title/body, anchored source, and diff
together. Questions, `nit` comments, terse alternatives, and human-confirmed
naming/style/documentation/API/configuration corrections qualify. Downstream clustering determines reuse; do not require
severity, recurrence, or broad generalizability here.

Reject unsupported speculation, no concrete correction, source that did not change as requested, generated churn,
merges, reverts, or evidence depending on unavailable intent/runtime facts. Do not pre-filter solely by the title or
discussion wording.

`POSITIVE` means the problematic form and uses the before/first-parent revision. `NEGATIVE` means the
corrected/compliant form and uses the after/correcting revision. Every POSITIVE range intersects a removed line and
every NEGATIVE range intersects an added line in the exact canonical diff. Emit both labels for replacements when both
sides exist; emit only the changed side for addition/deletion-only corrections. Do not encode absent code as a range.
Ranges are one-based, nonempty, and within the actual historical file.

`fileRevision.path` is relative to the Git repository root, even when the inspected project is a subdirectory.
Resolve that root with `git rev-parse --show-toplevel`. Preserve the full path from the corresponding diff side
(without its `a/` or `b/` prefix); for renames, use the before path for POSITIVE and the after path for NEGATIVE.
Use that same repository-relative path in revision reads and the idempotency key.

Read the full canonical diff as evidence using
`git --no-pager diff --no-color --no-ext-diff --unified=200 <before> <after> -- <before-path> <after-path>` output, or
the equivalent exact-revision diff reader result. If unavailable, block the item.

Persist each PR Signal's relevant unified-diff excerpt in at most **4000 characters total**, including file and hunk
headers and newlines. Select the change that supports this Signal and the surrounding context needed to understand it;
omit unrelated files, hunks, changes, and distant context. Keep source lines verbatim, their ` ` / `-` / `+` prefixes,
the original before/after paths, and original line coordinates. You may split a large hunk into smaller contiguous
excerpts: recalculate each hunk's start and line counts for the retained lines, without renumbering the source. A side
with zero lines names the preceding line (`@@ -10,0 +11,1 @@` for an insertion after old line 10). Every expected range
must still intersect a retained change on its label's side. Do not cut a diff at character 4000, clip a source line,
insert ellipses, or substitute a summary. If a faithful excerpt cannot fit, report that Signal as failed rather than
silently losing supporting evidence. The verifier checks the excerpt's source bytes and coordinates against the full
canonical diff, so the excerpt need not equal the complete diff.

Each persisted record has this shape:

```json
{
  "id": "s-<10 lowercase hex>",
  "idempotencyKey": "<stable immutable provenance key>",
  "fileRevision": {
    "path": "src/Example.kt",
    "revision": "<full evidence revision>",
    "expectedRanges": [
      {
        "start": 1,
        "end": 1
      }
    ]
  },
  "source": {},
  "label": "POSITIVE",
  "description": "One line explaining what this range demonstrates",
  "syntheticExampleId": null,
  "provenance": {
    "workItemId": "<stable item ID>"
  }
}
```

Construct the idempotency key from immutable repository/source identity, work-item ID, a deterministic signal index,
label, path, revision, and ranges. Use `s-` plus the first 10 lowercase SHA-256 hex characters as the ID. Do not include
a transient run/plan ID in this key. PR Signal drafts carry neither key nor ID: the server derives both. If a provider
supplies an immutable analysis batch ID, preserve it in provenance.
Keep mutable orchestration progress in the MCP plan.

PR source preserves `type: "FromPR"`, `prNumber`, complete `title`, ordered complete human `discussionMessages`,
focused `diffPositiveToNegative` (at most 4000 characters), and discussion `url`. A PR chunk worker does not write it: it
stores Signal drafts naming the diff's revisions and paths, and optionally related diff lines away from the ranges. The
server completes this source from the prepared work item, cutting the canonical diff to the file's changes, or, when they
do not fit, to the changes around the draft's ranges and related lines.

Commit source preserves `type: "FromCommit"`, the full correcting `commitRevision`, and `url` when available. The commit
object determines its parent, message, and canonical diff; do not duplicate them in the Signal. The correcting
`commitRevision` is the same for both labels; `fileRevision.revision` selects the actual evidence side.

Keep descriptions concise and source metadata in its defined fields. Do not include rule names, rule IDs,
language/severity judgments, proposed implementations, or synthetic examples in this extraction stage.

Signal writes reject malformed records and inconsistencies in IDs, revisions, paths, or changed-line ranges before
persistence, returning the artifact and failing field. Correct the candidate from verified evidence and recompute its
key/ID when provenance changes; select a faithful bounded excerpt rather than altering source evidence to pass
validation. Server validation checks consistency of supplied evidence; workers must still verify it against the
repository or PR provider.
