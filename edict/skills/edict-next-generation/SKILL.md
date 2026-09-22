---
name: edict-next-generation
description: Run isolated generation workers for every frozen cluster target.
---

# Edict Next Generation Orchestration

Load only this skill. Do not edit the worktree or call cluster-processing MCPs yourself.

For every Qodana MCP call, pass the inspected IntelliJ project as `projectPath`. Never pass the Edict worktree as
`projectPath`; the worktree is repository data already loaded in the run context.

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
Return when all workers have returned. The stage budget is 660 minutes; wait in chunks of at most 60 minutes and stop at
the deadline.
