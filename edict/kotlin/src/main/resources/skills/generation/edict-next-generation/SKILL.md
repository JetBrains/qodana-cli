---
name: edict-next-generation
description: Managed coordinator subagent that runs isolated generation workers for every frozen cluster target.
---

# Edict Next Generation Orchestration

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use it for every child delegation and task transition.

Load only this skill. Do not edit the worktree or call cluster-processing MCPs yourself.

The prompt supplies the absolute worktree path and generation scratch root. Resolve both before the first MCP call or worker
launch. Stop if the scratch root equals or is below the worktree; otherwise create it and store all transient output there.

Call `edict_next_get_generation_clusters`. This MCP returns the clusters to process and `maxConcurrentClusterTasks` value.

Keep up to `maxConcurrentClusterTasks` workers active. Launch one fresh native `spawn_agent` worker per cluster with
`edict-next-cluster-generation` in the first prompt line. Pass its `clusterId`, `clusterDirectory`, worktree, inspected project,
and a unique private scratch directory below the generation scratch root. Launch the next cluster whenever a worker returns;
after all clusters have started, wait for the remaining workers.

Do not modify or repair repository changes made by workers. If you recognize any issue, flag it in the result for the
parent agent. A worker may leave its cluster Pending or mark it Invalid because infrastructure/tooling or its cluster
input/state is broken; neither is a stage failure.
Return when all workers have returned.
