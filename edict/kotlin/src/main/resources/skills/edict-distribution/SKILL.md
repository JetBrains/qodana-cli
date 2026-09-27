---
name: edict-distribution
description: Sequentially assign prepared inbox Signals to durable clusters through managed distribution transitions.
---

# Edict Distribution

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-distribution`. Start the
task and use only the three distribution tools below. Do not call generic state-write or state-delete tools. The
delegated `inbox.delete`, `cluster.write`, and `cluster.signal.write` capability authorizes the server-owned transition;
the preparation receipt binds this task to its selected inbox batch.

`edict_next_next_signal` returns the complete incoming Signal; use it as the decision authority. Its same-language
embedding neighbours are comparison candidates, not proof of compatibility.

Decide from the Signals: **can the incoming Signal and every current member be handled by the same IntelliJ
inspection?** Assign only when yes and languages match. Positive and negative Signals may share a cluster when they
define the boundary of that inspection. Otherwise create a new cluster with a provisional lowercase kebab-case id.
Do not split or rename an existing cluster. Read exact-revision source through local Git or the inspection server when
recorded evidence is ambiguous.

Repeat until `edict_next_next_signal` returns `STOP_DISTRIBUTION`:

1. Call `edict_next_next_signal` with your task token and the complete unchanged preparation receipt. Read the complete
   returned Signal. Repeated calls return the same current Signal until it has a durable assignment.
2. For every plausible existing cluster, call `edict_next_get_distribution_context` with `kind: "cluster"` and compare
   every member Signal, including negatives. Retrieve a useful neighboring inbox Signal with `kind: "signal"` when
   needed. A cluster read records the context receipt required for an existing-cluster assignment.
3. Choose one compatible existing cluster id or a new provisional id.
4. Call `edict_next_add_signal_to_cluster` with `signalId` and `clusterId`. This single transition preserves the exact
   Signal, updates/creates the Pending cluster and history, verifies the durable destination, then removes the inbox
   copy. If `added` is false, use the summary to correct the choice and retry the same Signal; do not request another
   Signal while it remains unassigned.

Return the assigned Signal IDs and affected cluster IDs. Finish only after distribution returns
`STOP_DISTRIBUTION`; the coordinator then validates the unchanged preparation receipt with
`edict_validate_distribution`.
