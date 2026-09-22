---
name: edict-next-code-example
description: Reduce one Edict Next Signal and its exact source revision to a validated code example.
---

# Edict Next Code Example

Load only this skill.

For every Qodana MCP call, pass the inspected IntelliJ project as `projectPath`. Never pass the Edict worktree as
`projectPath`; the worktree is repository data already loaded in the run context.

The prompt supplies exactly three absolute paths:

- `Signal path`: the Signal JSON to update.
- `Synthetic examples directory`: the target cluster's `synthetic-examples/` directory.
- `Inspected IntelliJ project`: the project to pass as `projectPath` in Qodana MCP calls.

You may change only files below the supplied examples directory and `syntheticExampleId` in the supplied Signal. Do not
change any other Signal field or any other file.

Your operation is `reduce(Signal, exact source revision) -> one self-contained code example`. A Signal is exact local
evidence; its example is a semantic program slice, not a rule hypothesis. Do not read cluster metadata, a candidate
inspection, or review feedback. Neither the reduction nor its label may depend on a general rule or Inspection KTS limits.

Read the supplied Signal. If it already has `syntheticExampleId`, return that ID. Otherwise retrieve its exact
`fileRevision` with `mcp__qodana__file_at_ref`, using `radius: 20`, and treat that source as the authority. Confirm that
the numbered response contains every requested expected-range line. If it does not, retry with `radius: 5` and then
`radius: 0`; do not create or change an example until the target lines are visible. Identify the construct covered by
`fileRevision.expectedRanges`, its diagnostic role, and the properties and relationships that make the Signal positive
or negative. The Signal description explains why the evidence was selected, but does not justify adding absent facts.

Delete irrelevant statements and declarations while preserving the diagnostic target and every PSI/semantic relation needed for
its exact positive or negative meaning. For a Signal about variable V in method M, retain V's declaration, relevant reads, writes,
aliases, and conditions/control flow affecting V; retain the types and members needed to resolve those operations and relevant
enclosing method/class modifiers, annotations, generics, hierarchy, and overload contracts. Apply the same dependency reasoning
to other targets. Replace external dependencies with minimal same-file declarations, interfaces, or stubs only when they preserve
the compile-time/PSI contracts and label-determining semantics. Do not generalize the Signal, weaken types, move the semantic
target, or invent facts to explain its classification. When unsure whether a detail matters, preserve it.

A positive example must contain exactly one reportable problem. Its sole expected range must identify the same semantic target
as the original source range; supporting declarations, usages, and control flow may appear elsewhere in the example. A negative
example must contain exactly one focused allowed case corresponding to the original source range, no reportable problems, and
an empty expected-range list. Do not create a trivial negative by removing the construct being evaluated.

For an unassigned Signal, create its example at a new path:

```plaintext
<synthetic-examples-directory>/<example-id>/metadata.json
<synthetic-examples-directory>/<example-id>/project/<file-name>.kt|java
```

Metadata contains `id`, `fileName`, `label`, and `expectedRanges`. Derive the expected ranges from the finished reduced source;
do not retain source line numbers after moving the target. Never copy the complete production source file.

Derive `clusterId` from the canonical examples-directory path and call
`mcp__qodana__edict_next_validate_code_example(clusterId, exampleId)` for the newly created example before assigning it.
Repair structural issues in the example and validate again after any
repair. The MCP does not establish semantics: independently confirm that the example demonstrates the Signal and that its
label and ranges are correct.

Only after both checks succeed, set the supplied Signal's `syntheticExampleId` and return the assigned example ID.
