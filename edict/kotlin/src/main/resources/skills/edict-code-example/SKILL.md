---
name: edict-code-example
description: Create or reuse one source-faithful synthetic example and assign it to a managed signal through scoped edict-mcp writes.
---

# Code Example

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-code-example`. This
leaf may write only the scoped examples directory and, when granted, `syntheticExampleId` in one persisted signal.

Start the task. The parent supplies the cluster ID, source project, private scratch path, and either a state-relative
signal path or a transient signal in private scratch. Read the full signal and retrieve its exact historical file/ranges
with local Git or a read-only source tool. Do not substitute the current checkout.

Read existing example metadata/source through MCP. Reuse only an example that faithfully represents the same semantic
case and label. Otherwise create one small, self-contained source file: a positive must contain exactly one instance of
the problem; a negative must contain none. Include only necessary declarations, never a complete production source file
or a hidden dependency on support files.

The persisted layout is `clusters/<id>/synthetic-examples/<example-id>/metadata.json` and `project/<file-name>.kt|java`.
Metadata contains `id`, `fileName`, `label`, and `expectedRanges`. Positive metadata has exactly one one-based range
covering the problem; negative metadata has an empty range list. Check valid source syntax with an available
parser/compiler using private scratch, then independently check semantic fidelity, label, and ranges. Do not use the
legacy IntelliJ Edict validator, which owns a different state session. If a required source/parser check cannot run,
fail with that limitation instead of declaring the example validated.

Persist the source and metadata using MCP, and read them back to verify hashes and structural consistency. Only then
assign `syntheticExampleId`. For a persisted signal, reread it and write the updated JSON through MCP, preserving every
other field. For a transient weak signal, return the example ID and update only the supplied scratch signal; never
create a persisted cluster signal from it. Finish with the example ID, paths, hashes, and validation result.
