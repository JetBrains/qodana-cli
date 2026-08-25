---
name: edict-next-run
description: Orchestrate an explicitly started Edict Next run from worktree creation through clustering, bounded per-cluster generation, final validation, commit, and push.
---

# Edict Next Run

The prompt supplies the Edict source repository, workspace, inspected Ultimate project, project revision, and concurrency limit.
Own the filesystem and Git workflow. MCP snapshots and validates state but does not create worktrees, edit files, commit, or push.

## Final-validation MCP contract

`mcp__qodana__edict_next_validate_repository`

- Input: none; it validates the worktree registered by the clustering prepare call.
- Output: `success`, `summary`, and zero or more `issues`, each with `path` and `message`.
- Effect: read-only worktree validation against the captured Signal baseline and the accumulated examples. It records whether the session has most recently passed final validation, reports a short statistics table to stdout, and writes `edict-next-verification.csv` to the Qodana results directory.
- Next action: publish only after a response with `success: true`. Any repair invalidates the practical result, so call this operation again before commit and push.

## Workflow

1. Create a Git worktree below the supplied workspace from the current source-repository revision. Create a branch named `edict-next/YYYY-MM-DD`; if it exists locally or remotely, append `-2`, `-3`, and so on. Make every repository change in this worktree.
2. Invoke `$edict-next-clustering` with the worktree and workspace. It produces a lineage-blind validated partition and a lossless evidence catalog without changing the worktree. Stop the run if partition validation fails.
3. After the evidence catalog and generation-task files are complete, remove the old `clusters/` contents and the JVM Signal files selected from `inbox/`. Leave non-JVM inbox Signals untouched. Keep `inspections/` until reuse decisions finish. Launch one task per validated partition group and explicitly invoke `$edict-next-cluster-generation` with its generation-task JSON. Allow at most eight tasks to run concurrently. Each task assigns its cluster identity, ancestry, and description, then owns only that cluster and the inspection file it selects.
4. After all tasks finish, resolve duplicate rule IDs, then make `inspections/` contain exactly one `<ruleId>.inspection.kts` for each `Generated` cluster and no orphan inspection files.
5. Call `mcp__qodana__edict_next_validate_repository`. For cluster-local failures, launch bounded repair tasks that preserve all Signals and examples, change the cluster to `NotGenerated`, clear inspection metadata, remove its candidate and inspection files, and append the reason to `history.md`. Revalidate. For missing/duplicate Signals or bad ancestry, abort without publishing; automated global repair is a TODO.
6. Only after successful final validation, commit every worktree change and push the new branch to the source repository's configured remote. Do not publish an unvalidated state.

Do not modify the inspected Ultimate project. Do not add or run tests. Do not modify the source checkout outside the worktree.
