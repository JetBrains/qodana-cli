---
name: edict-next-run
description: Orchestrate an Edict Next run from run start and worktree creation through clustering, bounded per-cluster generation, final validation, commit, and push.
---

# Edict Next Run

Start the run yourself and take the Edict source repository, workspace, inspected Ultimate project, project revision, and concurrency limit from the start response. A prompt may repeat these parameters, but the response is authoritative.
Own the filesystem and Git workflow. MCP snapshots and validates state but does not create worktrees, edit files, commit, or push.

## Start-of-run MCP contract

`mcp__qodana__edict_next_start_run`

- Input: optionally `sourceRepositoryPath`; omit it to use the Edict repository configured for the Qodana process.
- Output: `sourceRepository`, `workspaceRoot`, `projectPath`, `projectRevision`, `maxConcurrentClusterTasks`, and `tookOverPreviousRun`.
- Effect: registers a fresh run against the open Qodana project and creates the agent workspace. Nothing is read from or written to the Edict source repository. Starting always takes over, so `tookOverPreviousRun: true` only reports that an interrupted session left a run behind; it is not an error and requires no cleanup.
- Next action: create the worktree below `workspaceRoot` and use these parameters for the rest of the run. Every other `edict_next` operation fails with "No Edict Next run is active" until this call succeeds.

## Final-validation MCP contract

`mcp__qodana__edict_next_validate_repository`

- Input: none; it validates the worktree registered by the clustering prepare call.
- Output: `success`, `summary`, and zero or more `issues`, each with `path` and `message`.
- Effect: read-only worktree validation against the captured Signal baseline and the accumulated examples. It records whether the session has most recently passed final validation, reports a short statistics table to stdout, and writes `edict-next-verification.csv` to the Qodana results directory.
- Next action: publish only after a response with `success: true`. Any repair invalidates the practical result, so call this operation again before commit and push.

## Completion MCP contract

`mcp__qodana__edict_next_validate_completion`

- Input: none; it revalidates the same worktree after publication.
- Output: `success`, `summary`, and zero or more `issues`, in the same shape as final validation.
- Effect: read-only. It confirms that the published state is exactly the state that passed final validation. A run that never passed final validation returns `success: false` instead of failing.
- Next action: report the summary and every issue. A response with `success: false` means the published state is not the validated one and the run failed, even though the branch was pushed.

## Workflow

1. Call `mcp__qodana__edict_next_start_run`. Stop the run if it fails; no other operation can succeed without it.
2. Create a Git worktree below `workspaceRoot` from the current revision of `sourceRepository`. Create a branch named `edict-next/YYYY-MM-DD`; if it exists locally or remotely, append `-2`, `-3`, and so on. Make every repository change in this worktree.
3. Invoke `$edict-next-clustering` with the worktree and workspace. It produces a lineage-blind validated partition and a lossless evidence catalog without changing the worktree. Stop the run if partition validation fails.
4. After the evidence catalog and generation-task files are complete, remove the old `clusters/` contents and the JVM Signal files selected from `inbox/`. Leave non-JVM inbox Signals untouched. Keep `inspections/` until reuse decisions finish. Launch one task per validated partition group and explicitly invoke `$edict-next-cluster-generation` with its generation-task JSON. Run at most `maxConcurrentClusterTasks` tasks concurrently. Each task assigns its cluster identity, ancestry, and description, then owns only that cluster and the inspection file it selects.
5. After all tasks finish, resolve duplicate rule IDs, then make `inspections/` contain exactly one `<ruleId>.inspection.kts` for each `Generated` cluster and no orphan inspection files.
6. Call `mcp__qodana__edict_next_validate_repository`. For cluster-local failures, launch bounded repair tasks that preserve all Signals and examples, change the cluster to `NotGenerated`, clear inspection metadata, remove its candidate and inspection files, and append the reason to `history.md`. Revalidate. For missing/duplicate Signals or bad ancestry, abort without publishing; automated global repair is a TODO.
7. Only after successful final validation, commit every worktree change and push the new branch to the source repository's configured remote. Do not publish an unvalidated state.
8. Call `mcp__qodana__edict_next_validate_completion` and report its result.

Do not modify the inspected Ultimate project. Do not add or run tests. Do not modify the source checkout outside the worktree.
