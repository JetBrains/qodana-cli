# Strong-signal evidence and storage

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

Canonical diff content is verbatim
`git --no-pager diff --no-color --no-ext-diff --unified=200 <before> <after> -- <before-path> <after-path>` output, or
the equivalent exact-revision diff reader result. Never synthesize/truncate hunks, normalize content, copy a different
item's diff, or insert an availability message. If unavailable, block the item.

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
a transient run/plan ID in this key. If a provider supplies an immutable analysis batch ID, preserve it in provenance.
Keep mutable orchestration progress in the MCP plan.

PR source preserves `type: "FromPR"`, `prNumber`, complete `title`, ordered complete human `discussionMessages`,
canonical `diffPositiveToNegative`, and discussion `url`.

Commit source preserves `type: "FromCommit"`, full correcting `commitRevision`, `parentRevision`, complete `message`,
canonical `diffPositiveToNegative`, and `url` when available. The correcting commitRevision is the same for both labels;
`fileRevision.revision` selects the actual evidence side.

Keep description concise; store messages/diffs/source metadata in their own fields. Do not include rule names, rule IDs,
language/severity judgments, proposed implementations, or synthetic examples in this extraction stage.

Signal writes reject malformed records and inconsistencies in IDs, revisions, paths, or changed-line ranges before
persistence, returning the artifact and failing field. Correct the candidate from verified evidence and recompute its
key/ID when provenance changes; do not alter the canonical diff to make validation pass. Server validation checks
consistency of supplied evidence; workers must still verify it against the repository or PR provider.
