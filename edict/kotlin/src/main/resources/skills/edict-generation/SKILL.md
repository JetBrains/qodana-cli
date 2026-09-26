---
name: edict-generation
description: Run isolated managed generation workers for every frozen cluster target and collect durable outcomes.
---

# Edict Generation

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-generation`. Coordinate
only; do not edit state.

Start your task. Resolve the state and private generation scratch roots using the managed protocol. Keep orchestration
logs, manifests, prompts, result tables, and summaries in scratch, never in managed state. Read the selected cluster
descriptions and freeze a distinct set of Pending IDs before launching workers. If the parent explicitly requests all
Pending clusters, discover them with MCP reads. Return success for an empty set.

For each target create a nested `edict-cluster-generation` task. Grant `cluster.write`, `cluster.signal.write`,
`example.write`, and `inspection.write` only under its exact cluster directory, candidate/current inspection paths, and
an existing predecessor inspection path when necessary. Never grant the whole clusters directory to a single-cluster
worker. Pass a unique scratch directory, source project, cluster ID, and its child capability to a fresh native
subagent.

With the configured 50-agent capacity, keep **up to 15 cluster workers running concurrently** whenever targets
remain. Fill all available cluster slots before waiting. Reserve three shared coordinator frames (manager, run,
generation) and up to three frames per active cluster (cluster, reviewer, review's example child), leaving two spare.
Each cluster must run its own children one at a time; include that constraint in its assignment. Do not fill the
remaining slots with more cluster workers and leave no room for their descendants. If the host exposes a smaller
capacity, reduce the number of active clusters to preserve those reservations.

When any worker finishes, verify its persisted outcome, close/dispose its native agent and completed descendants,
and immediately launch the next queued cluster in that slot. Do not wait for other clusters to finish their repairs.
Each cluster has at most three review iterations including the initial candidate; Pending after exhausting that budget
is a valid bounded outcome and must release its slot for the next target.

After every frozen target has been started, wait for the remaining workers and verify their persisted tasks. Require
exact coverage of the frozen set with no missing or duplicate cluster outcomes. Do not modify or repair worker
artifacts; flag issues for the parent. A valid Pending or Invalid cluster outcome is not a stage failure. A failed
worker, an unperformed check claimed as successful, or a broken persisted artifact fails orchestration. Return each
cluster ID, status, accepted inspection path if any, and the Pending/Invalid reason from history, then finish.
