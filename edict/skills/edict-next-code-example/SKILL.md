---
name: edict-next-code-example
description: Assign a validated existing or new code example to one Edict Next Signal JSON.
---

# Edict Next Code Example

Load only this skill.

For every Qodana MCP call, pass the inspected IntelliJ project as `projectPath`. Never pass the Edict worktree as
`projectPath`; the worktree is repository data already loaded in the run context.

The prompt supplies exactly two absolute paths:

- `Signal path`: the Signal JSON to update.
- `Synthetic examples directory`: the target cluster's `synthetic-examples/` directory.

You may change only files below the supplied examples directory and `syntheticExampleId` in the supplied Signal. Do not
change any other Signal field or any other file.

Read the complete Signal and retrieve its exact `fileRevision` with `mcp__qodana__file_at_ref`. Treat that source as the
authority. Identify the source construct covered by `fileRevision.expectedRanges`, its diagnostic role, and the properties
and relationships that make the Signal positive or negative.

The assigned example is a small, self-contained reduction of that exact evidence. Prefer deleting unrelated code and replacing
dependencies with minimal declarations. Do not reinterpret or generalize the Signal, move the diagnostic target to another
element, or change a classification-relevant modifier, annotation, type relationship, assignment, call, or control-flow
condition. When unsure whether a detail affects the label, preserve it.

A positive example must contain exactly one reportable problem. Its sole expected range must identify the same semantic target
as the original source range; supporting declarations, usages, and control flow may appear elsewhere in the example. A negative
example must contain exactly one focused allowed case corresponding to the original source range, no reportable problems, and
an empty expected-range list. Do not create a trivial negative by removing the construct being evaluated.

Reuse an example only when it already satisfies these invariants and represents the same semantic case, target, and label.
Otherwise, create an example at:

```plaintext
<synthetic-examples-directory>/<example-id>/metadata.json
<synthetic-examples-directory>/<example-id>/project/<file-name>.kt|java
```

Metadata contains `id`, `fileName`, `label`, and `expectedRanges`. Never copy the complete production source file.
Keep required declarations in one self-contained source file: support files do not participate in validation or inspection
execution.

Derive `clusterId` from the canonical examples-directory path and call
`mcp__qodana__edict_next_validate_code_example(clusterId, exampleId)`, including for a reused example. Repair structural
issues and validate again. The MCP does not establish semantics: independently confirm that the example demonstrates the
Signal and that its label and ranges are correct.

Only after both checks succeed, set the supplied Signal's `syntheticExampleId` and return the assigned example ID.
