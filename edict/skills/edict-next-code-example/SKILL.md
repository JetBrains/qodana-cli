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

Read the complete Signal and retrieve its exact `fileRevision` with `mcp__qodana__file_at_ref`. Compare that evidence with
the existing examples. The assigned example must be small and self-contained. A positive example must contain exactly one
occurrence of the problem described by the Signal; a negative example must contain none. Reuse an example only when it
already satisfies this invariant and genuinely represents the same semantic case and label. Otherwise, create an example at:

```plaintext
<synthetic-examples-directory>/<example-id>/metadata.json
<synthetic-examples-directory>/<example-id>/project/<file-name>.kt|java
```

Metadata contains `id`, `fileName`, `label`, and `expectedRanges`. A positive example must declare exactly one one-based
target range covering its sole problem occurrence; a negative example must use an empty range list. Never copy the complete
production source file. Include only the declarations needed to represent the Signal in one self-contained source file:
support files do not participate in validation or inspection execution.

Derive `clusterId` from the canonical examples-directory path and call
`mcp__qodana__edict_next_validate_code_example(clusterId, exampleId)`, including for a reused example. Repair structural
issues and validate again. The MCP does not establish semantics: independently confirm that the example demonstrates the
Signal and that its label and ranges are correct.

Only after both checks succeed, set the supplied Signal's `syntheticExampleId` and return the assigned example ID.
