---
name: edict-next-run
description: Managed coordinator subagent that orchestrates the Edict Next distribution and generation stages.
---

# Edict Next Run

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use it for every child delegation and task transition.

Load only this skill. Launch every stage with the native `spawn_agent` tool and mention its skill only in the first line
of the fresh worker prompt. Do not edit the worktree, call cluster-processing MCPs, or run tests yourself.

When an inspection tool accepts `projectPath`, pass the inspected IntelliJ project. Never pass the Edict worktree;
the worktree is repository data already loaded in the run context.

The prompt supplies the Edict worktree, inspected project, and shared workspace for generation scratch data.

Run sequentially:

1. Launch `edict-next-distribution` for up to 120 minutes with the worktree and inspected project paths. Distribution
   prepares the pipeline snapshot before assigning Signals. After it returns, call
   `edict_next_validate_distribution` and stop on failure.
2. Choose a unique absolute generation scratch root below the supplied workspace and outside the worktree. Pass the worktree path,
   generation scratch root, and inspected project to `edict-next-generation`, then launch it for up to 660 minutes.
3. Call the read-only `edict_next_validate_generation` for up to 40 minutes. Stop unless it returns `PUBLISH`.

After successful validation, collect every Invalid cluster id and its recorded infrastructure/tooling or cluster-state failure
from `history.md`. Commit and push the worktree, then let the Qodana script finish. Pending and Invalid clusters do not block
publication. In the final response, list every Invalid cluster and its recorded reason. Wait for workers and MCP calls in
chunks of at most 60 minutes and stop on failure or timeout. The complete session budget is 900 minutes.
