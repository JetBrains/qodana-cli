---
name: edict-next-generation
description: Run isolated generation workers for every frozen cluster target.
---

# Edict Next Generation Orchestration

Load only this skill. Ask to load the `edict-next-cluster-generation` skill in the first line of a fresh worker prompt. Do not edit
the worktree or call cluster-processing MCPs yourself.

For every Qodana MCP call, pass the inspected IntelliJ project as `projectPath`. Never pass the Edict worktree as
`projectPath`; the worktree is repository data already loaded in the run context.

The prompt supplies the absolute worktree path and an absolute generation scratch root. Resolve both paths before the first MCP call
or worker launch. Stop with failure if the scratch root equals the worktree or is below it. Create the scratch root when it does not exist.
Store all orchestration logs, prompts, result tables, summaries, and other transient files below this scratch root. Never store
them in the worktree.

Call `edict_next_get_generation_clusters`. This MCP returns the clusters to process and `maxConcurrentClusterTasks` value.

Keep up to `maxConcurrentClusterTasks` workers active, launching the next cluster whenever any worker returns. Launch one fresh worker
per cluster using **native** spawn_agent tool. Pass its `clusterId`, `clusterDirectory`, the worktree path, a unique private 
scratch directory below the generation scratch root, and the inspected project path. Whenever any worker returns, 
immediately launch the next unstarted cluster without waiting for the other active workers. After every cluster 
has been started, wait for the remaining workers to return.

Do not modify or repair repository changes made by workers. If you recognize any issue, flag it in the result for the
parent agent. A worker may leave its cluster Pending or mark it Invalid because infrastructure/tooling or its cluster
input/state is broken; neither is a stage failure.
Return when all workers have returned. The stage budget is 695 minutes; wait in chunks of at most 60 minutes and stop at
the deadline.
