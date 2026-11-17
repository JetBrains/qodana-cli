---
name: edict-code-example-overseer
description: Managed subagent that reconciles one Edict cluster's code examples with all Signals before inspection selection.
---

# Edict Code Example Overseer

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use its assigned task lifecycle.

Load only this skill. The prompt supplies `Cluster id`. Resolve the target cluster below the `stateDirectory` of your
run context and use its `projectDirectory` as the inspected IntelliJ project. Do not request or repeat the state
path in a prompt.

Reconcile from the Signals alone: do not read cluster history, `cluster.json`, candidate or predecessor inspections,
review artifacts, or other clusters.

Change examples only through `edict_save_code_example`, `edict_assign_code_example`, and
`edict_delete_code_example`, with your own task token for an overseer repair.

Each code example stands for one case of the cluster's rule: one way a problem occurs, or one reason a problem is absent.
Several Signals of one case share its example: assign the existing example to such a Signal rather than reducing another one.
A strong Signal may get an example no strong Signal referenced yet; the example then becomes strong.

## Map the new Signals

First call `edict_prepare_signal_mapping(token, clusterId)`. When it returns new Signals and at least one example, create
and delegate exactly one `edict-signal-mapping` managed task, named `Map Signals for <clusterId>`, to a fresh native
subagent, following the manager protocol, with only:

```text
Load the edict-signal-mapping skill.

Cluster id: <clusterId>
Mapping directory: <returned directory>
```

Wait for it to complete. Its lines are hints for the reconciliation below, not decisions; confirm each against the exact
sources before acting on it. Compare the label of each mapped Signal with the label of its example:

- the same label: the example is the candidate for the Signal's case; when it does stand for that case, assign it with
  `edict_assign_code_example` instead of reducing a new example;
- the opposite label: a contradiction to look into. The Signal, the example or the mapping is wrong: check the example
  against the exact source of the Signals assigned to it, and the new Signal against its own;
- `none`: the Signal needs an example of its own.

When the worker fails, reconcile without hints.

## Reconcile the evidence

List every Signal in the cluster with `edict_get_cluster_signals` and read its examples with
`edict_get_cluster_examples`. Reconcile the Signals one at a time: read a Signal's full evidence from
`<signalsDirectory>/<id>.json` when you reconcile it. Resolve the Git root from
the inspected project and retrieve each exact `fileRevision` with read-only Git
(`git show <revision>:<repository-relative-path>`), treating those complete historical bytes as authoritative. If an
object is unavailable, read it with `edict_file_at_ref` anchored at the expected range,
with radius 20, then 5 and 0, until every requested expected-range line is visible. Do not reconcile that Signal before that. Independently infer
the broadest coherent rule that explains all positive and negative Signals. Use that inference only to identify which
source relationships are relevant to each Signal reduction. Do not persist a rule, id, name, or description, and do not
account for an inspection candidate or Inspection KTS implementation limits.
If exact Signals are incompatible, still reconcile each Signal independently and report the exact contradiction; this
is a semantic result, not an example-generation failure.

A Signal without an assigned example has nothing to inspect yet: reconcile it when you review the example created for
it, or when you reduce it yourself, not before delegating it. Until then use only its index entry.

For every Signal with an assigned example:

1. Identify the construct covered by its expected range and the source facts that make its label correct, wherever they
   are: in the range, elsewhere in its file, or in declarations it depends on in other files.
2. Inspect its assigned example. Keep it when it is a faithful, focused semantic slice of the exact source.
3. Delete every incorrect example with `edict_delete_code_example`; this unassigns its Signals.

Then handle every Signal without an example. When at most three Signals are without an example of their case, reduce each yourself
from its exact source, as [the code-example skill](../edict-code-example/SKILL.md) describes, avoiding the defect
of any example you deleted for it, and assign it with `edict_assign_code_example`; do not delegate them. Otherwise
split the Signals without an example, in Signal ID order, into groups of at most five, and for each group create and
delegate one `edict-code-example` managed task to a fresh native subagent, following the manager protocol. Give it
only this cluster id, the group's Signal ids and, for each Signal whose example you deleted, one line
`Rejected example <signal id>: <defect>`. State the defect as the source facts the deleted example dropped, changed or
invented, compared with the exact source revision, for example a missing field write or a changed declaring type; never
a rule, a label opinion or an inspection concern. The worker takes configured paths from its run context. Wait for
every example worker and require its managed task to complete; do not reduce a delegated Signal inline except as the
last resort below. Delegate each Signal to a worker at most twice. When the second worker leaves it without an example
or its example is incorrect too, correct that example yourself with `edict_save_code_example`, or create and assign
it when missing, and report the Signal with both defects. Preserve the original types, modifiers, annotations,
hierarchy, assignments, calls, aliases, and control flow that determine the label. Replace unrelated dependencies with
minimal same-file declarations only when their PSI and semantic contracts remain equivalent.
A positive example contains exactly one reportable occurrence and one expected range over the same semantic target as
the source Signal. A negative example contains one focused allowed occurrence and an empty expected-range list. Keep
each example in one self-contained source file; support files do not participate in validation.

After reconciling the complete corpus, call `edict_validate_cluster_examples(clusterId)`. Repair every
reported issue and repeat until it succeeds: a reference that does not resolve in the example's module is a defect of
that example. Independently review every created or changed example against
its exact source revision because structural validation cannot establish source fidelity.

Examples not referenced by a `STRONG` cluster Signal are weak. Make sure no weak example contradicts a strong Signal:
delete the complete directory of an example no cluster Signal references when its label contradicts a strong Signal on
the same semantic case, for example a weak negative containing the construct a strong positive reports. Keep every
other weak example as it is.

Return a concise summary listing every Signal and its example id, created or changed examples, deleted weak examples
with the strong Signal each contradicted, and any evidence that prevented complete reconciliation.
