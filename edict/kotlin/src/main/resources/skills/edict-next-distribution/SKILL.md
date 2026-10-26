---
name: edict-next-distribution
description: Managed subagent that sequentially assigns prepared inbox Signals to durable clusters.
---

# Edict Next Distribution

Run only as a delegated managed subagent. Follow [the managed task protocol](../edict_manager/references/task-protocol.md) before
domain work and use its assigned task lifecycle.

Load only this skill.

Call `edict_next_calculate_neighbours_by_embeddings` once before the loop. It also consumes re-published inbox Signals
already owned by exactly one cluster, replaces the clustered model, records that replacement in history, and moves the
cluster to Pending. Include the reported replacement count in the task result. A task that continues the run's batch
gets "already calculated" and nothing changes.

## Evidence

`edict_next_next_signal` returns the complete incoming Signal; use it as the decision authority. Nearest embedding
neighbors are comparison candidates, not proof of compatibility.

Decide from the Signals: **can the incoming Signal and every current member be handled by the same IntelliJ
inspection?** Assign only when yes and languages match.
Positive and negative Signals may share a cluster when they define the boundary of that inspection. Otherwise create a
new cluster with a provisional lowercase kebab-case id. Do not split an existing cluster; splitting is manual.

## Loop

Call `edict_next_next_signal` once to get the first Signal. Then repeat until a response's action is
`STOP_DISTRIBUTION` or `HAND_OFF`:

1. Read the complete current Signal.
2. Compare every member Signal, including negatives, of every plausible existing cluster. The nearest small cluster
   candidates already list their `signals`, which counts as their context; retrieve `kind: "cluster"` context for any
   other plausible cluster. Inbox candidates show their label, path and description; retrieve one with `kind: "signal"`,
   or read its exact revision with `edict_file_at_ref`, only when that is not enough.
3. Choose one compatible existing cluster id or a new provisional id.
4. Call `edict_next_add_signal_to_cluster` with `signalId` and `clusterId`. Existing-cluster assignment requires its
   context receipt. If `added` is false, use the summary to correct the choice and retry the same Signal. Otherwise its
   `next` is the next Signal, `HAND_OFF` or `STOP_DISTRIBUTION`; do not call `edict_next_next_signal` for it.

Return after `STOP_DISTRIBUTION` or `HAND_OFF`, with the number of Signals this task assigned. On `HAND_OFF` this task's
share of the batch is done: finish it `completed` and report the remaining count from its summary.
