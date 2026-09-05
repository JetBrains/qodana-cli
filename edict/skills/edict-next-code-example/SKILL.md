---
name: edict-next-code-example
description: Assign a validated existing or new code example to one Edict Next Signal JSON.
---

# Edict Next Code Example

Load only this skill.

The prompt supplies exactly two absolute paths:

- `Signal path`: the Signal JSON to update.
- `Synthetic examples directory`: the target cluster's `synthetic-examples/` directory.

You may change only files below the supplied examples directory and `syntheticExampleId` in the supplied Signal. Do not
change any other Signal field or any other file.

Read the complete Signal and retrieve its exact `fileRevision` with `mcp__qodana__file_at_ref`. Compare that evidence with
the existing examples. Reuse an example only when it genuinely represents the same semantic case and label. If none fits,
create a small, self-contained example at:

```plaintext
<synthetic-examples-directory>/<example-id>/metadata.json
<synthetic-examples-directory>/<example-id>/project/<file-name>.kt|java
```

Metadata contains `id`, `fileName`, `label`, and `expectedRanges`. Positive examples require exact one-based target ranges;
negative examples normally use an empty range list. Use one self-contained source file: support files do not participate in
validation or inspection execution.

Derive `clusterId` from the canonical examples-directory path and call
`mcp__qodana__edict_next_validate_code_example(clusterId, exampleId)`, including for a reused example. Repair structural
issues and validate again. The MCP does not establish semantics: independently confirm that the example demonstrates the
Signal and that its label and ranges are correct.

Only after both checks succeed, set the supplied Signal's `syntheticExampleId` and return the assigned example ID. If the
work cannot be completed, leave the Signal unchanged and return a clear reason.
