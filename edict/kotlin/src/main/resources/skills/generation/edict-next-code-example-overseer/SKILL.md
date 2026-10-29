---
name: edict-next-code-example-overseer
description: Managed subagent that reconciles one Edict Next cluster's code examples with all Signals before inspection selection.
---

# Edict Next Code Example Overseer

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use its assigned task lifecycle.

Load only this skill. The prompt supplies `Cluster id`. Call `edict_context` with your own token, then resolve the target cluster below its
`stateDirectory` and use its `projectDirectory` as the inspected IntelliJ project. Do not request or repeat the state
path in a prompt.

Reconcile from the Signals alone: do not read cluster history, `cluster.json`, candidate or predecessor inspections,
review artifacts, or other clusters.

Change examples only through `edict_next_save_code_example`, `edict_next_assign_code_example`, and
`edict_next_delete_code_example`, with your own task token for an overseer repair.

## Reconcile the evidence

Read every Signal in the cluster and its examples with `edict_next_get_cluster_examples`. Resolve the Git root from
the inspected project and retrieve each exact `fileRevision` with read-only Git
(`git show <revision>:<repository-relative-path>`), treating those complete historical bytes as authoritative. If an
object is unavailable, read it with `edict_file_at_ref` anchored at the expected range,
with radius 20, then 5 and 0, until every requested expected-range line is visible. Do not reconcile that Signal before that. Independently infer
the broadest coherent rule that explains all positive and negative Signals. Use that inference only to identify which
source relationships are relevant to each Signal reduction. Do not persist a rule, id, name, or description, and do not
account for an inspection candidate or Inspection KTS implementation limits.
If exact Signals are incompatible, still reconcile each Signal independently and report the exact contradiction; this
is a semantic result, not an example-generation failure.

For every Signal:

1. Identify the construct covered by its expected range and the source facts that make its label correct.
2. Inspect its assigned example when present. Keep it when it is a faithful, focused semantic slice of the exact source.
3. Delete every incorrect example with `edict_next_delete_code_example`; this unassigns its Signals. When at most three
   Signals are without an example, reduce each yourself from the exact source you already read, as
   [the code-example skill](../edict-next-code-example/SKILL.md) describes, avoiding the defect of any example you deleted
   for it, and assign it with `edict_next_assign_code_example`; do not delegate them. Otherwise split the Signals
   without an example, in Signal ID order, into groups of at most five, and for each group create and delegate one
   `edict-next-code-example` managed task to a fresh native subagent, following the manager protocol. Give it only this
   cluster id, the group's Signal ids and, for each Signal whose example you deleted, one line
   `Rejected example <signal id>: <defect>`. State the defect as the source facts the deleted example dropped, changed
   or invented, compared with the exact source revision, for example a missing field write or a changed declaring
   type; never a rule, a label opinion or an inspection concern. The worker resolves configured paths with
   `edict_context`. Wait for every example worker and require its managed task to complete; do not reduce a delegated
   Signal inline except as the last resort below. Delegate each Signal to a worker at most twice. When the second
   worker leaves it without an example or its example is incorrect too, correct that example yourself with
   `edict_next_save_code_example`, or create and assign it when missing, and report the Signal with both defects.
   Preserve the original types, modifiers, annotations, hierarchy, assignments, calls, aliases, and control flow that
   determine the label. Replace unrelated dependencies with minimal same-file declarations only when their PSI and
   semantic contracts remain equivalent.
A positive example contains exactly one reportable occurrence and one expected range over the same semantic target as
the source Signal. A negative example contains one focused allowed occurrence and an empty expected-range list. Keep
each example in one self-contained source file; support files do not participate in validation.

After reconciling the complete corpus, call `edict_next_validate_cluster_examples(clusterId)`. Repair every
reported structural issue and repeat until it succeeds. Independently review every created or changed example against
its exact source revision because structural validation cannot establish source fidelity.

Examples not referenced by a `STRONG` cluster Signal are weak. Make sure no weak example contradicts a strong Signal:
delete the complete directory of an example no cluster Signal references when its label contradicts a strong Signal on
the same semantic case, for example a weak negative containing the construct a strong positive reports. Keep every
other weak example as it is. Do not assign weak examples to strong Signals.

Return a concise summary listing every Signal and its example id, created or changed examples, deleted weak examples
with the strong Signal each contradicted, and any evidence that prevented complete reconciliation.
