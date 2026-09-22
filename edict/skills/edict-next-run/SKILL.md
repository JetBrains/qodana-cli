---
name: edict-next-run
description: Orchestrate the Edict Next preparation, distribution, and generation stages.
---

# Edict Next Run

Load only this skill. Launch every stage with the native `spawn_agent` tool and mention its skill only in the first line
of the fresh worker prompt. Do not edit the worktree, call cluster-processing MCPs, or run tests yourself.

For every Qodana MCP call, pass the inspected IntelliJ project as `projectPath`. Never pass the Edict worktree as
`projectPath`; the worktree is repository data already loaded in the run context.

Treat the supplied workspace as the shared parent directory for the run. Pass the source repository only to preparation.

Run sequentially:

1. Launch `edict-next-prepare` for up to 35 minutes with the source repository, workspace, and inspected project. Retain
   the returned worktree path.
2. Launch `edict-next-distribution` for up to 120 minutes with the worktree and inspected project paths. After it returns, call
   `edict_next_validate_distribution` and stop on failure.
3. Choose a unique absolute generation scratch root below the supplied workspace and outside the worktree. Pass the worktree path,
   generation scratch root, and inspected project to `edict-next-generation`, then launch it for up to 660 minutes.
4. Call the read-only `edict_next_validate_generation` for up to 40 minutes. Stop unless it returns `PUBLISH`.

After successful validation, collect every Invalid cluster id and its recorded infrastructure/tooling or cluster-state failure
from `history.md`. Commit and push the worktree, then let the Qodana script finish. Pending and Invalid clusters do not block
publication. In the final response, list every Invalid cluster and its recorded reason. Wait for workers and MCP calls in
chunks of at most 60 minutes and stop on failure or timeout. The complete session budget is 900 minutes.
