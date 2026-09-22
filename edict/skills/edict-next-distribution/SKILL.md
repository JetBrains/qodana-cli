---
name: edict-next-distribution
description: Sequentially assign prepared inbox Signals to durable clusters.
---

# Edict Next Distribution

Load only this skill. Do not read another Edict Next skill or edit the worktree directly. The stage budget is 120 minutes.

Pass the inspected IntelliJ project as `projectPath` in every Qodana MCP call, never the Edict worktree.

## Evidence

`edict_next_next_signal` returns the complete incoming Signal; use it as the decision authority. Nearest embedding
neighbors are comparison candidates, not proof of compatibility.

Decide from the Signals: **can the incoming Signal and every current member be handled by the same IntelliJ
inspection?** Assign only when yes and languages match.
Positive and negative Signals may share a cluster when they define the boundary of that inspection. Otherwise create a
new cluster with a provisional lowercase kebab-case id. Do not split an existing cluster; splitting is manual.

## Loop

Repeat until `edict_next_next_signal` returns `STOP_DISTRIBUTION`:

1. Call `edict_next_next_signal` and read the complete Signal.
2. For every plausible existing cluster, retrieve `kind: "cluster"` context and compare every member Signal, including
   negatives. Retrieve a useful neighboring inbox Signal with `kind: "signal"`, or read its exact revision with
   `get_file_at_ref` when needed.
3. Choose one compatible existing cluster id or a new provisional id.
4. Call `edict_next_add_signal_to_cluster` with `signalId` and `clusterId`. Existing-cluster assignment requires its
   context receipt. If `added` is false, use the summary to correct the choice and retry the same Signal; do not request
   the next Signal while it remains unassigned.
Return only after distribution is complete.
