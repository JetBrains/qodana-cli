---
name: edict-next-code-example-overseer
description: Reconcile one Edict Next cluster's code examples with all of its Signals before inspection selection.
---

# Edict Next Code Example Overseer

Load only this skill. The prompt supplies:

- `Cluster directory`: the absolute target cluster directory.
- `Inspected IntelliJ project`: the project to pass as `projectPath` in Qodana MCP calls.

For every Qodana MCP call, pass the inspected IntelliJ project as `projectPath`. Never pass the Edict worktree as
`projectPath`.

You may change only `syntheticExampleId` in Signals below the supplied cluster directory and create, update, repair, or
delete files below its `synthetic-examples/` directory. Do not read cluster history, `cluster.json`, candidate or
predecessor inspections, review artifacts, or other clusters.

## Reconcile the evidence

Read every Signal in the cluster and retrieve every exact `fileRevision` with `mcp__qodana__file_at_ref`, using
`radius: 20`. Confirm that each numbered response contains every requested expected-range line. If it does not, retry
with `radius: 5` and then `radius: 0`; do not reconcile that Signal until the target lines are visible. Independently infer
the broadest coherent rule that explains all positive and negative Signals. Use that inference only to identify which
source relationships are relevant to each Signal reduction. Do not persist a rule, id, name, or description, and do not
account for an inspection candidate or Inspection KTS implementation limits.
If exact Signals are incompatible, still reconcile each Signal independently and report the exact contradiction; this
is a semantic result, not an example-generation failure.

For every Signal:

1. Identify the construct covered by its expected range and the source facts that make its label correct.
2. Inspect its assigned example when present. Keep it when it is a faithful, focused semantic slice of the exact source.
3. Create a missing example, or update an existing one when the complete Signal set reveals that its reduction omitted
   or retained context incorrectly. Preserve the original types, modifiers, annotations, hierarchy, assignments, calls,
   aliases, and control flow that determine the label. Replace unrelated dependencies with minimal same-file
   declarations only when their PSI and semantic contracts remain equivalent.
A positive example contains exactly one reportable occurrence and one expected range over the same semantic target as
the source Signal. A negative example contains one focused allowed occurrence and an empty expected-range list. Keep
each example in one self-contained source file; support files do not participate in validation.

After reconciling the complete corpus, call `mcp__qodana__edict_next_validate_cluster_examples(clusterId)`. Repair every
reported structural issue and repeat until it succeeds. Independently review every created or changed example against
its exact source revision because structural validation cannot establish source fidelity.

Examples not referenced by a cluster Signal are weak review evidence. When a coherent rule was inferred, keep one only
when its code and label are compatible with that rule; otherwise delete its complete example directory. Do not assign
weak examples to strong Signals.

Return a concise summary listing every Signal and its example id, created or changed examples, deleted weak examples,
and any evidence that prevented complete reconciliation.
