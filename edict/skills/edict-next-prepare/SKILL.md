---
name: edict-next-prepare
description: Create the Edict worktree and prepare retrieval for one Edict Next run.
---

# Edict Next Preparation

Load only this skill. Do not read or invoke another Edict Next skill. Create a worktree below the supplied workspace from
the source repository's current revision; modify neither the source checkout nor the inspected project. The new branch should follow
`edict-next/YYYY-MM-DD[-n]` format.

For every Qodana MCP call, pass the inspected IntelliJ project as `projectPath`. Never pass the Edict worktree as
`projectPath`; the worktree is repository data already loaded in the run context.

Call `mcp__qodana__edict_next_prepare_pipeline(worktreePath)` once. Return the worktree path and complete MCP response.
