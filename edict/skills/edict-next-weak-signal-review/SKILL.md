---
name: edict-next-weak-signal-review
description: Independently review one Edict Next inspection's sampled project findings, classify every finding from source evidence, and create validated negative code examples for confident false positives. Use only as a fresh worker launched by an Edict Next cluster-generation task.
---

# Edict Next Weak Signal Review

Load only this skill. When an example is needed, do not load `$edict-next-code-example`; mention it only in a fresh
worker prompt.

## Task contract

Accept exactly four absolute paths from the prompt:

- `Sampled findings JSON`: read-only input produced by `mcp__qodana__edict_next_get_new_inspection_results`.
- `Review output path`: the worker's only workspace output file.
- `Cluster directory`: read the cluster and add only validated negative examples below its `synthetic-examples/` directory.
- `Inspected project`: retrieve each sampled source revision from this repository.

Do not edit the candidate inspection, cluster Signals, description, history, or inspected project. Do not store sampled findings as Signals.

The findings input contains `ruleId`, `inspectionDescription`, `language`, and an indexed `findings` list. For every finding, inspect its exact revision and relevant range with `mcp__qodana__file_at_ref`. Read enough surrounding code and resolve PSI or symbols when needed to understand whether the inspection's general rule applies.

Classify each finding as:

- `TP`: the reported code genuinely violates the intended general rule.
- `FP`: the code must not be reported by that rule.
- `UNCERTAIN`: required source, semantic, or rule evidence is unavailable or genuinely ambiguous.

Do not infer a classification from path, API name, wording, or the inspection description alone. Do not mark the review complete from partial source retrieval. Include every input index exactly once.

For every confident `FP`, invoke `$edict-next-code-example` with the finding's file revision, the negative label, and a precise semantic explanation. Create or reuse one negative example in the supplied cluster and validate it through MCP. Keep examples created from transient findings unattached to Signals. Do not create an example for `UNCERTAIN` evidence.

Write `Review output path` with exactly this shape:

```json
{
  "status": "COMPLETE|INCOMPLETE",
  "entries": [
    {"index": 0, "classification": "TP|FP|UNCERTAIN", "description": "evidence-based explanation", "exampleId": null}
  ],
  "createdExampleIds": ["example-id"],
  "summary": "concise review outcome"
}
```

Use `COMPLETE` only when every input finding is `TP` or `FP` and every `FP` has a semantically correct MCP-validated example, whether newly created or already represented. Use `INCOMPLETE` if any entry is `UNCERTAIN`, any source lookup failed, any index is missing, or example validation could not be completed. A `COMPLETE` review with no newly created examples tells the parent generation task that this iteration found no new negative evidence.
