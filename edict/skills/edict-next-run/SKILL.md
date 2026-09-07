---
name: edict-next-run
description: Orchestrate the Edict Next preparation, distribution, and generation stages.
---

# Edict Next Run

Load only this skill. Mention each stage skill only as the first line of a fresh worker prompt. Do not edit the worktree,
call cluster-processing MCPs, or run tests yourself.

For every Qodana MCP call, pass the inspected IntelliJ project as `projectPath`. Never pass the Edict worktree as
`projectPath`; the worktree is repository data already loaded in the run context.

Treat the supplied workspace as the shared parent directory for the run. Retain it after preparation. Pass the source
repository only to the preparation worker.

Run sequentially:

1. Launch `edict-next-prepare` for up to 35 minutes. Pass the source repository, workspace, and inspected project supplied in the
   run prompt. It creates the worktree and prepares all required context for the pipeline. Retain both the supplied workspace path
   and the returned worktree path.
2. Launch `edict-next-distribution` for up to 120 minutes. After it returns, call
   `edict_next_validate_distribution` and stop on failure. Pass worktree path to the worker.
3. Choose a unique absolute generation scratch root below the supplied workspace and outside the worktree. Pass the worktree path,
   generation scratch root, and inspected project to `edict-next-generation`, then launch it for up to 695 minutes. Stop before
   launch if the scratch root equals the worktree or is below it.
4. Call the read-only `edict_next_validate_generation` for up to 40 minutes. Stop unless it returns `PUBLISH`.

After successful validation, collect every Invalid cluster id and its manual-repair reason from `history.md`. Commit and
push the worktree, then let the Qodana script finish. Pending and Invalid clusters do not block publication. In the final
response, list every Invalid cluster and its recorded reason. Wait in chunks of at most 60 minutes for stage workers and
MCP calls and stop on a failed stage or timeout. The complete session budget is 900 minutes.
