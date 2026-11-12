---
name: edict-signal-mapping
description: Managed subagent that tells, without seeing labels, which existing code example of an Edict cluster stands for the case of each new Signal.
---

# Edict Signal Mapping

Run only as a delegated managed subagent. Follow [the managed task protocol](../edict_manager/references/task-protocol.md) before
domain work and use its assigned task lifecycle.

Load only this skill. The prompt supplies `Cluster id` and `Mapping directory`. The directory holds `signals/<id>.json`, the id
and exact `fileRevision` of each new Signal, and `examples/<id>/`, the source file of each code example of the cluster. Labels,
expected ranges of the examples, Signal descriptions and sources are left out on purpose: map by the code alone. Do not read
anything else of the Edict state, the cluster, its Signals or examples' metadata, its history, or any inspection. Change nothing.

A code example stands for one case of the cluster's rule: one way a problem occurs, or one reason a problem is absent. Two
places are the same case when the same facts decide whether they are a problem, whatever their names and the code around them.
Each new Signal is one more place: find the example that stands for its case, or tell that none does.

For each Signal, in Signal id order:

1. Retrieve the exact `fileRevision` with read-only Git from the inspected project's Git root (`git show
   <revision>:<repository-relative-path>`). If the object is unavailable, read it with `edict_file_at_ref` anchored at the
   expected range, with radius 20, then 5 and 0, until every expected-range line is visible.
2. Identify the construct its expected ranges cover and the facts that decide whether it is a problem, wherever they are: in
   the range, elsewhere in the file, or in declarations it depends on.
3. Compare those facts with each example: its construct and the facts the example keeps around it. Textual similarity is not
   enough, and a difference only in names, formatting or unrelated code is not a different case. Do not guess whether a Signal
   or an example is a problem; only whether they are the same case.

Return exactly one line per Signal, and nothing else:

```text
<signal id> -> <example id>: <the deciding facts the Signal and the example share>
<signal id> -> none: <the deciding fact of the Signal that no example has>
```
