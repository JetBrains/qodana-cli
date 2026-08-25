---
name: edict-next-commit-strong-signal
description: Convert free-form human feedback and current code context into one verified positive or negative strong signal for a Qodana static-analysis rule, then commit it to the configured rules repository. Use only when explicitly installed and invoked for a Qodana manual-commit session; do not use for speculative analysis, previews, or automated signal discovery.
---

# Commit a Edict Next Strong Signal

Convert free-form user feedback into exactly one strong signal. Treat the explicit submission request as the human confirmation, and resolve the structured rule, label, evidence, and provenance from the feedback plus the current agent context.

## Required result

Derive and verify before submission:

- Precise evidence-specific signal description.
- `POSITIVE` or `NEGATIVE` label.
- Repository-relative file path and revision.
- One-based inclusive evidence range.
- Human explanation of what the evidence confirms.
- Source provenance: manual feedback, PR discussion, or commit.

The user does not need to provide these fields in a schema. Use the current file/selection, repository state, referenced PR or commit, and available tools to resolve them from free-form feedback. Ask one concise question only when a required value cannot be determined safely. Do not invent revisions, ranges, human statements, or source links.

## Workflow

1. Confirm that the invocation explicitly requests persistence. If it requests only analysis or a preview, show a draft and do not call the submission tool.
2. Interpret the free-form feedback into one concrete evidence description. Preserve the user's intended meaning and do not formulate or persist a rule; clustering creates rules later.
3. Infer the label from the user's wording and code context. Ask if both positive and negative interpretations remain plausible.
4. Resolve the relevant file, revision, and range from the current agent context or an explicitly referenced PR/commit using the available repository or Qodana revision tools.
5. Verify that the range exists and contains evidence relevant to the label:
   - `POSITIVE` confirms code that violates or should trigger the rule.
   - `NEGATIVE` confirms code that complies with or must not trigger the rule.
6. Require the absolute path of the locally available rules-repository checkout. Use an explicit invocation value first, then `QODANA_EDICT_REPOSITORY_PATH`. Never guess it.
7. Build the inbox record. Use `SubmittedFeedback` source metadata for direct free-form feedback, preserving the user's original statement as `problemMessage` and the exact selected code as `codeSnippet`. Preserve a source URL when the user explicitly identifies a PR or commit.
8. Construct an idempotency key from the original human statement, label, evidence path, revision, and range. Compute lowercase SHA-256 over its exact UTF-8 bytes; the signal ID is `s-` plus the first 10 hexadecimal characters.
9. Write the record to `inbox/<signal-id>.json`. Do not overwrite a different existing record.
10. Present a compact final preview containing the signal ID, label, source, revision, path, range, and explanation.
11. If persistence is confirmed, call `mcp__qodana__validate_inbox_changes` exactly once with this relative path. Use the managed session ID when one was supplied, otherwise use `manual`. Otherwise remove the draft file and ask for confirmation.
12. Stop without staging if validation fails. Recompute the file's SHA-256 and require it to match the receipt.
13. Inspect rules-repository status, stage only the validated path, and commit once. Never amend, push, or stage unrelated paths.
14. Finish after the local commit succeeds. Report the signal ID, validation receipt ID, and commit SHA. Pushing is a separate user or CI orchestration step.

## Rules-repository and commit policy

This policy is shipped as part of the skill and always applies, even when the private rules repository contains no instructions. If the checkout has applicable `AGENTS.md` files, also follow compatible stricter requirements; their absence never weakens this policy.

- Keep the rules repository on its current branch. Do not fetch, pull, switch branches, rebase, merge, stash, or modify remotes.
- `POSITIVE` means reportable violating code; `NEGATIVE` means compliant code that must not be reported.
- Record one evidence side per signal with an immutable full revision, minimal relevant one-based inclusive ranges, the original human provenance, and an evidence-specific description.
- Do not create or update clusters, synthetic examples, inspections, or unrelated repository metadata.
- Do not overwrite a different record. If the deterministic path already contains byte-identical content, report it as already persisted without committing.
- Create one non-empty commit with subject `Add Edict Next strong signal <signal-id>`. The commit must contain exactly the single validated path.
- Never amend or push. Do not use broad staging commands such as `git add .` or `git add -A`.

## Submission payload

```json
{
  "id": "s-<first 10 lowercase SHA-256 hex characters>",
  "idempotencyKey": "stable key derived from human source and evidence coordinates",
  "fileRevision": {
    "path": "repository/relative/File.kt",
    "revision": "full revision",
    "expectedRanges": [{"start": 10, "end": 12}]
  },
  "source": {
    "type": "SubmittedFeedback",
    "problemMessage": "original free-form human feedback",
    "codeSnippet": "exact selected source text",
    "url": "optional source URL"
  },
  "label": "POSITIVE",
  "description": "what this evidence confirms",
  "syntheticExampleId": null
}
```

This flat layout follows `clusters/*/signals/*.json`; `idempotencyKey` and optional orchestration `provenance` are inbox-only fields. Do not add a rule or proposed rule to the signal. When feedback refers to an existing inspection, preserve its actual `inspectionId`, `inspectionName`, and `inspectionDescription`; never synthesize those fields. The signal ID is deterministic; do not invent a random ID. MCP validates schema, containment, stable identity, duplicates, and file hashes. The agent owns file creation and all Git operations. Credentials must come from the process environment or configured Git credential helper and must never be printed or persisted.

## Refusal conditions

Do not commit when:

- The invocation did not explicitly request persistence.
- The supposed confirmation was generated only by an LLM.
- The rule depends on hidden domain intent or runtime information.
- Path, revision, or evidence range cannot be verified.
- The label remains ambiguous.
- The evidence does not support the proposed rule.

Explain the missing requirement and leave the store unchanged.
