---
name: edict-next-verification
description: Materialize terminal Edict decisions, roll back unfinished work, validate, and publish the worktree.
---

# Edict Next Verification

Load only this skill. Do not read or invoke any other Edict Next skill and do not perform free-form repository repair.
The prompt supplies the absolute UTC session deadline. Check it before every operation and stop when it is reached.

1. Call `mcp__qodana__edict_next_get_run_state` and require stage `VERIFICATION`.
2. Call `mcp__qodana__edict_next_validate_repository` exactly once. MCP materializes terminal decisions, restores every
   unfinished cluster and its routed Signals to the pre-run snapshot, writes verification output, and validates the
   resulting repository.
3. If validation does not return `PUBLISH`, stop without editing, committing, or pushing anything.
4. On `PUBLISH`, commit every worktree change and push its branch.
5. For a direct MCP run, call `mcp__qodana__edict_next_validate_completion` after push and require `FINISH_RUN`. A Qodana
   script run is revalidated automatically when its Codex session ends.

Do not modify the inspected project, add tests, or run tests.
