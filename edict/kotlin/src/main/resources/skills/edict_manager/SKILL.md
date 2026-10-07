---
name: edict_manager
description: Plan and coordinate root user requests for managed Edict signal analysis and inspection generation. Delegated workers with a task/token must use their assigned managed skill instead.
---

# Edict Manager

Turn the user's request into a short pipeline of registered managed skills. Execute every subtask in plan
in a fresh native subagent. You own planning and coordination; children own domain work.

This skill is for the root manager only. If you already received a delegated task/token, load the assigned worker's
`SKILL.md` from the supplied absolute path and follow that skill. Shared references under `edict_manager/references/`
are a protocol library, not an instruction to invoke this manager skill.

Read [the managed execution protocol](references/protocol.md) before starting. Require an available `edict-mcp` server.
The request carries no paths: call `edict_context` for them (see the protocol). The manager capability must never enter a child
prompt or inherited
conversation. An unavailable server or
native subagent runtime is a failed prerequisite, not permission to execute stages inline.

# Workflow

1. Read `edict_registry` for managed skills list. Use the registered skill IDs in MCP calls for scheduling. Do not load
   child skills yourself. Route bounded commit lists to `edict-batch-signal-analysis`, review discussions to
   `edict-pr-signal-analysis`, Git `-S`/`-G` searches for more evidence supporting selected stored Signals or clusters
   to `edict-git-history-signal-analysis`, and old-snapshot versus HEAD inspection comparisons to
   `edict-retrospective-signal-analysis`.
2. Use managed to build a list of steps required for fulfilling user request. Use availabale mcp tools if it's necessary
   to understand edict state.
3. Call `edict_plan_create(request, steps)` without a token, with the user's concrete
   request and ordered `{skill, title}` steps. The response contains `plan` and a private manager `token`; use that
   token for every subsequent manager lifecycle call. `edict_plan_get` is public, but may receive the token for caller
   attribution.
   Only one successful creation is allowed per server lifetime, even after all
   tasks finish. Never call it again once you have the token. After a server restart, claim an unfinished plan using
   its exact persisted request and ordered top-level skill/title steps; this returns the existing plan and a fresh
   token. Reuse pending/failed tasks, keep completed results, and avoid duplicate children.
   A new request after a terminal plan needs a new server. Keep the plan
   ID and state-relative path `plans/<id>.json`; never write the plan yourself.
5. Follow the protocol's assignment flow: supply `edict_delegate` the complete token-free task instructions starting
   with the child's exact managed skill invocation, then pass its returned short launch `prompt` to native
   `spawn_agent`.
   The worker fetches the full assignment with `edict_task_get`; do not copy task instructions into the launch message.
   Spawn without inherited conversation (`fork_turns: "none"`, or `fork_context: false` in runtimes exposing that
   parameter).
   Include relevant prior-stage results in the submitted instructions. Wait for its native completion; a wait that times
   out while the child still runs is not a reason to read the plan, so wait again. Read the plan once after the child
   completes to verify its persisted task is completed. Never print the child token or save it in a prompt file.
   Resolve the delegation's `skillPath` against the installed skills directory (the parent of this skill's directory).
   Include that absolute `SKILL.md` path in the submitted instructions and require reading it before `edict_task_start`.
   Explicit-only children may be absent from the runtime's skill catalog; the file path is authoritative.
6. Stop on any failed child or uncompleted task. Cancel a lost worker through `edict_task_cancel`, then cancel remaining
   unstarted dependent stages with an explicit upstream-failure reason. Do not launch them or turn skipped work into
   success. This leaves the plan terminal for a new request after server restart; a requested retry can still
   re-delegate failed stages in
   dependency order. Report the failed task and existing plan path so the user can inspect durable progress.
7. After all stages complete, read the persisted plan and report its path, the produced signal/cluster/inspection IDs,
   Generated inspections' `knownProblems`, and any Pending or Invalid clusters with their reasons. Completion means
   every planned task completed; a Generated inspection additionally requires the generation skill's evidence and
   review checks. The trusted host appends a USD report for each top-level stage including all descendant workers, every
   cluster-generation substage and inclusive cluster total, and the complete run total including the manager. It fetches
   the current official OpenAI Standard list rates and prices disjoint uncached-input, cached-input, cache-write, and
   output tokens per response. Reasoning is an output subset and must not be charged twice.

Do not commit, push, create state-repository worktrees, or perform external publication as part of this pipeline.
