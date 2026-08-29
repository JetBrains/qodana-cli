---
name: edict-next-code-example
description: Assign or create one mandatory Edict Next code example for a stored Signal or a transient reviewed project finding and verify its parsing, ranges, and PSI structure through MCP.
---

# Edict Next Code Example

## MCP contract

`mcp__qodana__edict_next_validate_code_example`

- Input: the absolute `exampleDirectory` inside a cluster's `synthetic-examples/` directory.
- Output: `success`, `summary`, structural `issues`, the resolved `targetPsiElements` for valid target ranges, and `nextAction`.
- Effect: read-only PSI validation of metadata, source presence, parsing, ranges, and target elements. It reports the result to stdout and does not decide whether the example is semantically representative.
- Next action: follow `nextAction`: repair and retry on `REPAIR_CODE_EXAMPLE`; on `CONTINUE_CLUSTER_GENERATION`, independently confirm the label, meaning, and expected ranges before returning to the caller.

Read the complete stored Signal or transient reviewed-finding payload and its cluster. For a Signal loaded from the cluster, preserve its declared strength. First look for an existing cluster example that genuinely represents the same semantic case and label. If one fits, set the stored Signal's `syntheticExampleId` to that example ID without duplicating it.

A false-positive project finding supplied by `$edict-next-weak-signal-review` is transient weak evidence. Use its file revision and semantic explanation only to create a new negative example; do not store it as a Signal or assign a `syntheticExampleId` to it.

Otherwise, obtain the referenced source with repository-context MCP calls such as `mcp__qodana__file_at_ref`. Choose a small self-contained example from it, or generate a new example when no real snippet is suitable. Store it as:

```text
synthetic-examples/<example-id>/metadata.json
synthetic-examples/<example-id>/project/<file-name>.kt|java
```

Metadata has `id`, `fileName`, `label`, and `expectedRanges`. Positive examples require exact one-based target ranges. Negative examples normally use an empty range list. The project directory may contain additional support files when necessary. For a stored Signal, update its JSON with `syntheticExampleId`; for a transient false-positive finding, leave the new example unattached.

Call `mcp__qodana__edict_next_validate_code_example` with the example directory. Repair all parsing, language, range, and PSI problems and validate again. MCP establishes structural feasibility only: independently ensure the code actually demonstrates the Signal and that its label and ranges are semantically correct.
