---
name: edict-next-prepare
description: Create the Edict worktree and prepare one protocol-compatible Edict Next run.
---

# Edict Next Preparation

Load only this skill. Do not read or invoke any other Edict Next skill.

The prompt supplies the source repository, workspace, inspected project, protocol version, and absolute UTC deadline.
Do not start work after that deadline or continue beyond it.

1. Create a worktree below the workspace from the source repository's current revision. Use branch
   `edict-next/YYYY-MM-DD`, adding `-2`, `-3`, and so on when needed. Modify neither the source checkout nor the
   inspected project.
2. Call `mcp__qodana__edict_next_prepare_pipeline(worktreePath, workspacePath, protocolVersion)` exactly once.
3. Require the returned protocol version to match the prompt. Return the worktree and complete MCP response. Only
   `PREPARE_BATCH` or `START_GENERATION` is successful. Do not create a distribution batch.

Do not distribute Signals, generate inspections, add tests, or run tests.
