---
name: edict-next-historical-rule-analysis
description: Enrich selected stored Qodana rules with human-confirmed commit evidence from a bounded Git range. Use only when explicitly invoked with stored rule IDs, a Qodana historical-analysis session ID, and the historical MCP tools.
---

# Edict Next Historical Rule Analysis

Search bounded repository history for corrective commits supporting selected stored rules. A stored rule comes exclusively from `clusters/<rule-id>/description.json` and has `id`, `description`, `language`, and `previousIds`. Do not derive the search rule from inbox signals, cluster signals, or a fresh analysis batch. Do not discover unrelated rules, rewrite or group rules, generate examples, or implement inspections.

The invocation must supply:

- A Qodana historical-analysis session ID.
- One or more stored rule IDs.
- One bounded Git revision expression.
- Optional candidate and accepted-evidence limits.
- The analyzed source project path used to route Qodana MCP calls.
- The absolute path of the local rules-repository checkout when evidence must be persisted.

Stop with a clear error if an input or required MCP tool is unavailable.

## Workflow

1. For every selected rule ID, read exactly `clusters/<rule-id>/description.json` from the supplied rules repository. Do not read its `signals` or `synthetic-examples` directories to formulate the rule or search.
2. Verify that each description has exactly the stored rule fields `id`, `description`, `language`, and `previousIds`, that its `id` matches the selected directory, and that order matches the invocation. Encode the descriptions as `rulesJson` and call `mcp__qodana__prepare_historical_search` with the session ID, `rulesJson`, revision range, and limits. Retain the exact `rulesJson`, range, and limits for final validation; preparation is stateless. If the MCP schema exposes `projectPath`, pass the analyzed source project path from the invocation.
3. Stop if preparation fails.
4. Process every returned rule query independently and in order:
   - Derive focused searches only from the stored description and language. Prefer API names, structural transformations, and corrective terms.
   - Use read-only shell Git commands to search the supplied revision range. Useful operations include `git log --grep`, `git log -S`, `git log -G`, `git log -- <path>`, `git show`, `git diff`, `git grep <pattern> <revision>`, `git ls-tree`, and `git blame`.
   - Bound every history traversal to the supplied revision range and maximum candidate count. Do not search unrelated refs or unbounded history.
    - For promising commits, resolve the commit revision, first parent, complete message, and unified diff. Retain each package locally with `commitRevision`, `parentRevision`, `message`, and `diff`; do not send candidates to MCP during discovery.
    - Exclude root commits, messages beginning with `Merge`, `Revert`, or `[automated]`, and commits without changed `java`, `kt`, `ts`, `js`, or `py` source outside paths containing `test/`, `build/`, or `generated/`.
    - Inspect every retained commit message and diff before deciding whether it independently confirms the same rule.
    - You may repeat discovery with different Git searches. Deduplicate retained candidates and never exceed the configured candidate limit in total for a query.
   - Produce exactly one terminal result for the query.
   - Use `FAILED` for a query-level tool failure, then continue with later queries.
5. Verify that query IDs, rule IDs, and ordering exactly match preparation.
6. Build `candidatesJson` as one ordered object per prepared query: `{"queryId":"...","candidates":[...]}`. Include every locally retained candidate used to calculate `candidatesInspected`, including candidates that produced no accepted evidence. Encode the complete result array as `resultsJson` and call `mcp__qodana__validate_historical_results` exactly once with the search ID, exact original `rulesJson`, range, limits, `candidatesJson`, and `resultsJson`. MCP recomputes the search and query IDs, validates all inputs together, returns a digest covering them, and retains nothing.
7. For every accepted evidence pair, write a `POSITIVE` inbox record for the parent-side evidence and a `NEGATIVE` inbox record for the correcting commit as `inbox/<signal-id>.json` in the rules repository. Prefix file-tool writes with the invocation's rules repository file-tool path exactly as supplied; never derive it from the canonical path. Use the canonical path only for rules-repository reads and Git commands. Follow the inbox storage contract below exactly.
8. Call `mcp__qodana__validate_inbox_changes` once with exactly the new files. Stop without staging if it fails. Verify every returned SHA-256 locally.
9. Stage only validated paths and commit once. Never amend, push, or stage unrelated paths.
10. Finish after validation and, when evidence was found, the local commit succeeds. Report rule IDs, receipt IDs, signal IDs, and commit SHA. Pushing is a separate orchestration step. MCP never writes a historical result file.

Use read-only Git in the analyzed source repository. Mutation is permitted only in the explicitly supplied rules repository after MCP validation. Inspect status first; stop if unrelated staged changes exist. Credentials must come from the process environment or Git credential helper and must never be printed or persisted.

The MCP `projectPath` field routes a tool call to an open Qodana project. For every Qodana MCP call, set it to the invocation's analyzed source project path. Never use the rules-repository path or temporary Codex workspace as `projectPath`; neither is the open analyzed project.

## Rules-repository and commit policy

This policy is shipped as part of the skill and always applies, even when the private rules repository contains no instructions. If the checkout has applicable `AGENTS.md` files, also follow compatible stricter requirements; their absence never weakens this policy.

- Keep the rules repository on its current branch. Do not fetch, pull, switch branches, rebase, merge, stash, or modify remotes.
- `POSITIVE` always means reportable violating code; `NEGATIVE` means compliant code that must not be reported. A historical corrective pair therefore stores the parent side as positive and the corrected side as negative.
- Store one signal per evidence side, with an immutable full revision, minimal relevant one-based inclusive ranges, human commit provenance, and an evidence-specific description.
- Do not create or update clusters, synthetic examples, inspections, or unrelated repository metadata.
- Do not overwrite a different record. Byte-identical deterministic records are already persisted and require no new commit.
- After MCP validation and hash verification, create at most one non-empty commit for the search with subject `Add Edict Next historical evidence (<historical-search-id>)`. The commit must contain exactly the validated paths.
- Never amend or push. Do not use broad staging commands such as `git add .` or `git add -A`.

## Evidence acceptance

Accept a candidate only when all conditions hold:

1. The human-authored commit describes or clearly performs a correction rather than a feature addition.
2. Its before/after transformation demonstrates the same precise structural rule, not merely shared keywords or APIs.
3. The violation is statically detectable without hidden domain intent or runtime information.
4. The parent revision contains the problematic form at an exact one-based inclusive range.
5. The commit revision contains the correction and both paths occur in the locally retained candidate package.
6. The commit is independent of the seed evidence and is not a revert, automated migration, or ambiguous broad refactor.

A similar repository occurrence without a human correction is weak evidence and must not be submitted here.

## Inbox storage contract

Use the flat signal layout established by `clusters/*/signals/*.json`. Historical evidence is still human-authored commit evidence, so its source type is `FromCommit`; historical-search identifiers are orchestration provenance and must not replace the source metadata.

Every historical signal must use this exact source shape:

```json
"source": {
  "type": "FromCommit",
  "commitRevision": "<correcting commit accepted by validate_historical_results>",
  "parentRevision": "<first parent accepted by validate_historical_results>",
  "message": "<complete human-authored commit message>",
  "diffPositiveToNegative": "<validated corrective diff>"
},
"provenance": {
  "ruleId": "<stored rule ID>",
  "historicalSearchId": "<search ID returned by prepare_historical_search>",
  "queryId": "<query ID returned by prepare_historical_search>"
}
```

All source and provenance fields shown above are required. The query ID belongs in `provenance.queryId`; never substitute `workItemId` or omit it.

Construct an idempotency key from immutable historical provenance: stored rule ID, historical search ID, query ID, label, path, revision, and ranges. Compute lowercase SHA-256 over the exact UTF-8 idempotency-key bytes and set `id` to `s-` plus its first 10 hexadecimal characters. Each record must otherwise follow this shape:

```json
{
  "id": "s-<10 lowercase hex>",
  "idempotencyKey": "<stable historical provenance key>",
  "fileRevision": {
    "path": "<repository-relative source path>",
    "revision": "<full immutable revision>",
    "expectedRanges": [{"start": 1, "end": 1}]
  },
  "source": {
    "type": "FromCommit",
    "commitRevision": "<correcting commit>",
    "parentRevision": "<first parent>",
    "message": "<complete commit message>",
    "diffPositiveToNegative": "<validated corrective diff>"
  },
  "label": "POSITIVE | NEGATIVE",
  "description": "<what this evidence side demonstrates>",
  "syntheticExampleId": null,
  "provenance": {
    "ruleId": "<stored rule ID>",
    "historicalSearchId": "<historical search ID>",
    "queryId": "<historical query ID>"
  }
}
```

Do not add `rule`, `ruleId`, or a proposed rule statement to the signal. Leave `syntheticExampleId` null because rules and examples are created only after clustering.

## Result contract

Every result includes `queryId`, `status`, and `candidatesInspected`.

For a completed search without qualifying evidence:

```json
{
  "queryId": "...",
  "status": "NO_EVIDENCE",
  "candidatesInspected": 3,
  "explanation": "brief bounded-search conclusion"
}
```

For accepted evidence:

```json
{
  "queryId": "...",
  "status": "EVIDENCE_FOUND",
  "candidatesInspected": 3,
  "limitReached": false,
  "evidence": [
    {
      "sourceType": "COMMIT",
      "positive": {
        "path": "repository-relative path from the candidate package",
        "revision": "parent revision from the candidate package",
        "ranges": [{"start": 1, "end": 1}],
        "description": "the violation demonstrated here"
      },
      "negative": {
        "path": "repository-relative path from the candidate package",
        "revision": "correcting commit from the candidate package",
        "ranges": [{"start": 1, "end": 1}],
        "description": "the compliant replacement demonstrated here"
      },
      "explanation": "why this independently confirms the same rule"
    }
  ]
}
```

For failure, use `FAILED` with `category`, non-empty `diagnostic`, and `retryable`; do not include evidence.

Do not invent query IDs, revisions, paths, or ranges. Derive signal IDs deterministically using the inbox storage contract above.

## Guarantees

- Preserve query cardinality, identity, and ordering.
- Analyze queries independently and continue after query-level failures.
- Submit every inspected candidate package with the final results; only evidence accepted by `validate_historical_results` is valid provenance.
- Deduplicate evidence within each query.
- Submit once per prepared search.
- Treat the successful MCP receipts and local Git commit as the durable outcome when evidence was found.
