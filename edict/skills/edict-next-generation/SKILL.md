---
name: edict-next-generation
description: Run isolated generation workers for every frozen cluster target.
---

# Edict Next Generation Orchestration

Load only this skill. Ask to load the `edict-next-cluster-generation` skill in the first line of a fresh worker prompt. Do not edit
the worktree or call cluster-processing MCPs yourself.

Call `edict_next_get_generation_clusters`. This MCP returns the clusters to process and `maxConcurrentClusterTasks` value.

Keep up to `maxConcurrentClusterTasks` workers active, launching the next cluster whenever any worker returns. Launch one fresh worker per
cluster, passing its `clusterId` and `clusterDirectory`, a unique private scratch directory below the workspace, and the inspected
project path. Whenever any worker returns, immediately launch the next unstarted cluster without waiting for the other active workers.
After every cluster has been started, wait for the remaining workers to return.

Do not modify or repair repository changes made by workers. If you recognize any issue, flag it in the result for the
parent agent. A worker may leave its cluster Pending or mark it Invalid for manual repair; neither is a stage failure.
Return when all workers have returned. The stage budget is 395 minutes; wait in chunks of at most 60 minutes and stop at
the deadline.
