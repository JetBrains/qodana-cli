---
name: edict-next-distribution
description: Orchestrate sequential Edict batch workers and validate every batch without performing distribution itself.
---

# Edict Next Distribution

Load only this skill. `$edict-next-batch` is opaque: do not load, read, summarize, or perform it. Mention it only as the
first literal line in a fresh worker prompt. Use native subagent tools only; never create shell or Codex wrappers.

The prompt supplies the authoritative run-state response and absolute UTC deadline. Do not read or edit the worktree
yourself. Check the time before each batch and worker launch. Wait in chunks of at most 60 minutes and never beyond the
deadline; interrupt an active worker and stop when it is reached.

While run state is `DISTRIBUTION`:

1. When `nextAction` is `PREPARE_BATCH`, call `mcp__qodana__edict_next_next_batch` and require `PROCESS_BATCH` or
   `START_GENERATION`. The MCP slices the frozen selection and resolves nearest Signal ids against current ownership.
2. On `PROCESS_BATCH`, launch exactly one fresh worker with this prompt:

   ```text
   $edict-next-batch

   Batch path: <batchPath>
   Edict worktree: <worktreePath>
   Inspected project: <inspectedProjectPath>
   ```

3. Wait for that worker. If it fails or times out, stop the run.
4. Call `mcp__qodana__edict_next_validate_batch` with the exact batch path. If validation fails, stop immediately; do
   not repair or relaunch the batch worker.
5. Call `mcp__qodana__edict_next_get_run_state` and require `PREPARE_BATCH`, then repeat. Never overlap workers.

On `START_GENERATION`, require run state `GENERATION` and finish. Do not load processing skills, generate inspections,
add tests, or run tests.
