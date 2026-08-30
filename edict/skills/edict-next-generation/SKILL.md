---
name: edict-next-generation
description: Orchestrate isolated per-cluster generation workers, retry incomplete workers once, and close generation.
---

# Edict Next Generation Orchestration

Load only this skill. `$edict-next-cluster-generation` is opaque: do not load, read, summarize, or perform it. Mention it
only as the first literal line in a fresh worker prompt. Use native subagent tools only; never create shell or Codex
wrappers and never edit the worktree.

Call `mcp__qodana__edict_next_get_run_state` and require stage `GENERATION`. For every returned cluster, create a private
scratch directory below the workspace and launch a fresh worker with exactly:

```text
$edict-next-cluster-generation

Cluster directory: <clusterDirectory>
Private scratch workspace: <unique scratch directory>
Inspected project: <inspectedProjectPath>
```

Run workers in waves of at most `maxConcurrentClusterTasks`. One failed cluster must not cancel unrelated workers. Wait
for the complete wave before refreshing run state, so MCP never scans cluster files while another worker is editing them.
Check UTC time around every wait; wait in chunks of at most 60 minutes and never beyond the supplied absolute deadline.
For every returned cluster:

- `COMPLETED` or `EXHAUSTED`: accept the result.
- Any other state: call `mcp__qodana__edict_next_record_cluster_task_failure` with the stable cluster id and concrete
  failure. On `RETRY_CLUSTER_TASK`, schedule one fresh worker with the same contract and a new scratch directory in the
  next wave.
- After the second failed attempt, leave the cluster for deterministic rollback.

At the deadline, interrupt active workers and record active or unstarted tasks as failures under the same two-attempt
rule. When every cluster is terminal, call
`mcp__qodana__edict_next_complete_generation` and require `VERIFY_REPOSITORY`. Do not add or run tests.
