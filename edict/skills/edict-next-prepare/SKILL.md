---
name: edict-next-prepare
description: Create the Edict worktree and prepare retrieval for one Edict Next run.
---

# Edict Next Preparation

Load only this skill. Do not read or invoke another Edict Next skill. Create a worktree below the supplied workspace from
the source repository's current revision; modify neither the source checkout nor the inspected project. The new branch should follow
`edict-next/YYYY-MM-DD[-n]` format.

Call `mcp__qodana__edict_next_prepare_pipeline(worktreePath)` once. The MCP loads and validates the repository,
and, when the inbox has JVM Signals, prepares the embedding environment and computes nearest neighbours for the first
100 alphabetical Signals. It exports the embedding cache and disposes the Python environment before returning.
Return the worktree and the complete MCP response. Do not do anything else.
