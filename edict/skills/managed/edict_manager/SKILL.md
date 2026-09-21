---
name: edict_manager
description: Plan and coordinate root user requests for managed Edict signal analysis and inspection generation. Delegated workers with a task/token must use their assigned managed skill instead.
---

# Edict Manager

Turn the user's request into a short pipeline of registered managed skills. Execute every stage and every nested subtask
in a fresh native subagent. You own planning and coordination; children own domain work.

This skill is for the root manager only. If you already received a delegated task/token, load the assigned worker's
`SKILL.md` from the supplied absolute path and follow that skill. Shared references under `edict_manager/references/`
are a protocol library, not an instruction to invoke this manager skill.

Read [the managed execution protocol](references/protocol.md) before starting. Require an available `edict-mcp` server,
the source project, and scratch space outside the registered state root. Obtain the manager capability from the first
successful `edict_plan_create` response. The manager capability must never enter a child prompt or inherited conversation. An unavailable server or
native subagent runtime is a failed prerequisite, not permission to execute stages inline.

1. Read `edict_registry` and the relevant state through `edict_list` / `edict_read`. Use the registered original skill
   IDs in MCP calls; use `$managed-edict-next-*` in child prompts. Do not load child skills yourself.
2. Explain the chosen high-level pipeline briefly. The default is `edict-next-batch-signal-analysis` followed by
   `edict-next-run`. For an existing-inbox request, use `edict-next-run` alone. For extraction only, use batch analysis
   alone. Do not invent a history range or PR selection when none is supplied; use existing inbox state or report the
   missing analysis input.
3. Read `edict_plan_get` first. Call `edict_plan_create(request, steps)` without a token, with the user's concrete
   request and ordered `{skill, title}` steps. The response contains `plan` and a private manager `token`; use that
   token for every subsequent manager call, including `edict_registry`, `edict_list`, `edict_read`, and `edict_plan_get`,
   so logs identify you as the caller. Only one successful creation is allowed per server lifetime, even after all
   tasks finish. Never call it again once you have the token. After a server restart, claim an unfinished plan using
   its exact persisted request and ordered top-level skill/title steps; this returns the existing plan and a fresh
   token. Reuse pending/failed tasks, keep completed results, and avoid duplicate children. If an unrelated plan is
   unfinished, report it without cancelling it. A new request after a terminal plan needs a new server. Keep the plan
   ID and state-relative path `plans/<id>.json`; never write the plan yourself.
4. For each stage in order, delegate its plan task with only the operations and path scope it needs. Batch analysis
   needs `inbox.write` on `inbox`; run needs `inbox.delete`, `cluster.write`, `cluster.signal.write`, `example.write`,
   and `inspection.write` on `inbox`, `clusters`, and `inspections`. Narrow these further when the user targets specific
   items. These are delegation ceilings, not permission for the orchestrator itself to edit state.
5. Spawn a fresh child without inherited conversation (`fork_turns: "none"`, or `fork_context: false` in runtimes
   exposing that parameter), its managed skill invocation, the returned child token/task ID/skill, explicit source inputs,
   scratch location, and any relevant prior-stage results. Wait for its native completion and verify its persisted task
   is completed through the plan. Never print the child token or save it in a prompt file.
   Resolve the delegation's `skillPath` against the installed skills directory (the parent of this skill's directory).
   Include that absolute `SKILL.md` path in the child prompt and explicitly require reading it before `edict_task_start`.
   Explicit-only children may be absent from the runtime's skill catalog; the file path is authoritative.
6. Stop on any failed child or uncompleted task. Cancel a lost worker through `edict_task_cancel`, then cancel remaining
   unstarted dependent stages with an explicit upstream-failure reason. Do not launch them or turn skipped work into
   success. This leaves the plan terminal for a new request after server restart; a requested retry can still re-delegate failed stages in
   dependency order. Report the failed task and existing plan path so the user can inspect durable progress.
7. After all stages complete, read the persisted plan and report its path, the produced signal/cluster/inspection IDs,
   and any Pending or Invalid clusters with their reasons. Completion means every planned task completed; a Generated
   inspection additionally requires the generation skill's evidence and review checks.

Do not commit, push, create state-repository worktrees, or perform external publication as part of this pipeline.
