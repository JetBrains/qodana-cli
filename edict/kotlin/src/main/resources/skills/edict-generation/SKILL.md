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

Run within the available native-agent capacity; when a worker completes, launch another. Wait for every started child
and verify its persisted task. Do not repair artifacts yourself. A valid Pending or Invalid domain outcome is reportable
without fabricating an inspection; a failed worker, unperformed required check, or broken persisted artifact fails this
orchestration task. Return IDs, statuses, accepted inspection paths, and recorded limitations, then finish.
