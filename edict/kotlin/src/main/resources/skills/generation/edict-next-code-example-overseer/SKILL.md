---
name: edict-next-code-example-overseer
description: Managed subagent that reconciles one Edict Next cluster's code examples with all Signals before inspection selection.
---

# Edict Next Code Example Overseer

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use its assigned task lifecycle.

Load only this skill. The prompt supplies:

- `Cluster directory`: the absolute target cluster directory.
- `Inspected IntelliJ project`: the project to pass as `projectPath` in Qodana MCP calls.

For every Qodana MCP call, pass the inspected IntelliJ project as `projectPath`. Never pass the Edict worktree as
`projectPath`.

You may change only `syntheticExampleId` in Signals below the supplied cluster directory and create, update, repair, or
delete files below its `synthetic-examples/` directory. Do not read cluster history, `cluster.json`, candidate or
predecessor inspections, review artifacts, or other clusters.

Managed state is read-only to filesystem tools. Delegated reducers persist and assign examples with
`edict_next_save_code_example` and `edict_next_assign_code_example`. For an overseer repair, use those same calls with
your own task token; delete only an unassigned example with `edict_next_delete_code_example`. Never use `apply_patch`
or shell writes below the cluster directory.

## Reconcile the evidence

Read every Signal in the cluster. Resolve the Git root from the inspected project and retrieve each exact
`fileRevision` with read-only Git (`git show <revision>:<repository-relative-path>`), treating those complete historical
bytes as authoritative. If an object is unavailable and Qodana exposes `file_at_ref`, use it with radius 20, then 5
and 0 until every requested expected-range line is visible. Do not reconcile that Signal before that. Independently infer
the broadest coherent rule that explains all positive and negative Signals. Use that inference only to identify which
source relationships are relevant to each Signal reduction. Do not persist a rule, id, name, or description, and do not
account for an inspection candidate or Inspection KTS implementation limits.
If exact Signals are incompatible, still reconcile each Signal independently and report the exact contradiction; this
is a semantic result, not an example-generation failure.

For every Signal:

1. Identify the construct covered by its expected range and the source facts that make its label correct.
2. Inspect its assigned example when present. Keep it when it is a faithful, focused semantic slice of the exact source.
3. For every missing or incorrect example, create and delegate one `edict-next-code-example` managed task to a fresh
   native subagent, following the manager protocol. Give it only the absolute Signal path, this cluster's synthetic
   examples directory, and the inspected IntelliJ project. Wait for every example worker and require its managed task
   to complete; do not perform the leaf reduction inline. Preserve the original types, modifiers, annotations,
   hierarchy, assignments, calls, aliases, and control flow that determine the label. Replace unrelated dependencies
   with minimal same-file declarations only when their PSI and semantic contracts remain equivalent.
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
