---
name: edict-next-run
description: Orchestrate the Edict Next preparation, distribution, and generation stages.
---

# Edict Next Run

Load only this skill. Mention each stage skill only as the first line of a fresh worker prompt. Do not edit the worktree,
call cluster-processing MCPs, or run tests yourself.

Run sequentially:

1. Launch `$edict-next-prepare` for up to 35 minutes. Pass the source repository, workspace, and inspected project supplied in the
   run prompt. It creates the worktree and prepares all required context for the pipeline.
2. Launch `$edict-next-distribution` for up to 120 minutes. After it returns, call
   `edict_next_validate_distribution` and stop on failure. Pass worktree path to the worker.
3. Launch `$edict-next-generation` for up to 395 minutes.
4. Call the read-only `edict_next_validate_generation` for up to 40 minutes. Stop unless it returns `PUBLISH`.

After successful validation, collect every Invalid cluster id and its manual-repair reason from `history.md`. Commit and
push the worktree, then let the Qodana script finish. Pending and Invalid clusters do not block publication. In the final
response, list every Invalid cluster and its recorded reason. Wait in chunks of at most 60 minutes for stage workers and
MCP calls and stop on a failed stage or timeout. The complete session budget is 600 minutes.
