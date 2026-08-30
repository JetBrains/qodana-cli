---
name: edict-next-run
description: Orchestrate the four isolated Edict Next stages without loading or performing any stage implementation.
---

# Edict Next Run

Load only this skill. Stage skills are opaque contracts: do not read, load, summarize, or perform them yourself. Mention
each stage skill only as the first literal line of a fresh subagent prompt. Use native subagent tools only; never launch
Codex through shell scripts or replace a prescribed worker prompt.

The Qodana prompt supplies the source repository, workspace, inspected project, concurrency, protocol version, and four
absolute UTC deadlines. For a direct standalone-MCP run, first call `mcp__qodana__edict_next_start_run` and use its
returned values and deadlines. Never recalculate or extend a deadline.

Run these agents sequentially:

1. Launch `$edict-next-prepare` with the source repository, workspace, inspected project, protocol version, and
   preparation deadline. Then call `mcp__qodana__edict_next_get_run_state`; continue only from `DISTRIBUTION` or
   `GENERATION`.
2. If distribution is required, launch `$edict-next-distribution` with the complete run-state response and distribution
   deadline. Re-read run state and require `GENERATION`.
3. Launch `$edict-next-generation` with the complete run-state response and generation deadline. Re-read run state and
   require `VERIFICATION`.
4. Launch `$edict-next-verification` with the complete run-state response, session deadline, and whether this is a direct
   MCP run. Re-read run state and require `PUBLICATION` for a Qodana-script run or `FINISHED` for a direct run.

Wait for each stage agent to finish before launching the next. Check UTC time around every wait; wait in chunks of at
most 60 minutes and never beyond the stage deadline. At a deadline, interrupt the stage agent and stop the run. Also stop
on a failed stage, protocol mismatch, or unexpected stage. Do not edit either repository, call processing MCPs, commit,
push, add tests, or run tests yourself.
