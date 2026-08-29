---
name: edict-next-run
description: Run Edict Next from run start through routing, generation, validation, commit, and push.
---

# Edict Next Run

The Qodana script prompt supplies the source repository, workspace, inspected project, and concurrency limit. When the
skill is invoked directly against the standalone Qodana MCP server instead, call
`mcp__qodana__edict_next_start_run` and use its returned parameters. Own the filesystem and Git workflow. Every MCP
response has an authoritative `nextAction`; follow it.

Keep `<workspace>/run-state.md` as the restart point. Record the worktree and branch, current batch path, remaining and
completed cluster ids, active scratch directories, last MCP response, and exact next step. Update it after every batch,
cluster-task result, and repository validation. Re-read it after interruption or context compaction.

1. Create a worktree below the supplied workspace from the source repository's current revision. Use branch
   `edict-next/YYYY-MM-DD`, adding `-2`, `-3`, and so on when needed. Make every repository change in this worktree.
2. Call `mcp__qodana__edict_next_prepare_pipeline(worktreePath, workspacePath)` exactly once. It validates timeout
   configuration, prepares retrieval, snapshots the original and rolling repository state, and returns the first
   alphabetical batch of up to 50 JVM Signals.
3. While `nextAction` is `PROCESS_BATCH`, launch exactly one fresh `$edict-next-batch` task with the returned
   `batchPath`, worktree, and inspected project. Never overlap batches. Continue with the next path it returns.
   `START_GENERATION` means the JVM inbox is empty. Stop the run if internal retrieval retries are exhausted.
4. Enumerate every `Pending` cluster after distribution. For each, create a private scratch directory below the
   workspace and launch a fresh `$edict-next-cluster-generation` task with only that cluster directory, its scratch
   directory, and the inspected project. Allow at most `maxConcurrentClusterTasks` tasks concurrently. Track every task
   to completion; do not let one cluster failure cancel unrelated clusters.
5. After all cluster tasks finish, call `mcp__qodana__edict_next_validate_repository`. It materializes recorded
   decisions, infers predecessor fallback for missing decisions, validates the complete worktree against the original
   snapshot, writes `edict-next-verification.csv`, and exports the embedding cache.
6. On `REPAIR_REPOSITORY`, repair only reported cluster-local problems and validate again. Abort on lost or multiply
   owned Signals or examples. Only `PUBLISH` permits commit and push.
7. Commit every worktree change and push the branch. For a direct MCP run, call
   `mcp__qodana__edict_next_validate_completion` afterward; `FINISH_RUN` completes the run, while
   `REPAIR_REPOSITORY` requires repair, validation, commit, push, and completion again. The Qodana script revalidates
   automatically when its Codex session ends. Hand back every cluster's status, history, and generated inspection.

Do not modify the inspected project or the source checkout outside the worktree. Do not add or run tests. Do
not publish a repository state that has not passed the final validation.
