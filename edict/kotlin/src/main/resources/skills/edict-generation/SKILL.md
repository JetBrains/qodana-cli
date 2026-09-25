---
name: edict-generation
description: Delegate one isolated managed generation worker per selected Pending cluster and collect durable outcomes.
---

# Edict Generation

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-generation`. Coordinate
only; do not edit state.

Start your task. Read the selected cluster descriptions and freeze a distinct set of Pending IDs. If the parent
explicitly requests all Pending clusters, discover them with MCP reads. Return success for an empty set.

For each target create a nested `edict-cluster-generation` task. Grant `cluster.write`, `cluster.signal.write`,
`example.write`, and `inspection.write` only under its exact cluster directory, candidate/current inspection paths, and
an existing predecessor inspection path when necessary. Never grant the whole clusters directory to a single-cluster
worker. Pass a unique scratch directory, source project, cluster ID, and its child capability to a fresh native
subagent.

With the configured ten-agent capacity, keep **two cluster workers running concurrently** whenever at least two
targets remain. Start both before waiting for either. Reserve three shared coordinator frames (manager, run,
generation) and up to three frames per active cluster (cluster, reviewer, review's example child), leaving one spare.
Each cluster must run its own children one at a time; include that constraint in its assignment. Do not fill the
remaining slots with more cluster workers and leave no room for their descendants. If the host exposes a smaller
capacity, reduce the number of active clusters to preserve those reservations.

When either worker finishes, verify its persisted outcome, close/dispose its native agent and completed descendants,
and immediately launch the next queued cluster in that slot. Do not wait for the other cluster to finish its repairs.
Each cluster has at most three review iterations including the initial candidate; Pending after exhausting that budget
is a valid bounded outcome and must release its slot for the next target.

Wait for every started child and verify its persisted task. Do not repair artifacts yourself. A valid Pending or Invalid domain outcome is reportable
without fabricating an inspection; a failed worker, unperformed required check, or broken persisted artifact fails this
orchestration task. Return IDs, statuses, accepted inspection paths, and recorded limitations, then finish.
