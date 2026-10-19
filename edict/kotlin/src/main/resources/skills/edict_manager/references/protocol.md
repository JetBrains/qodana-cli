# Managed execution protocol

This contract applies to the manager and every managed child. The first successful plan creator receives manager
authority; child write
capabilities are restricted to registered managed skills. Other skills may inspect source and create ordinary scratch
artifacts but receive no capability to modify Edict state.

## Authority and storage

The trusted host runs `qodana edict install` and `qodana edict mcp start` in the inspected project before the agent
starts. The first successful `edict_plan_create(request, steps)` call needs no token and returns `{plan, token}`. Its
caller becomes
manager; this is allowed only once per server lifetime, including after task completion. The manager claims it before
spawning children and keeps the returned token private. The host's agent sandbox must make the state
root read-only, keep parent conversations and token-bearing logs inaccessible to children, and leave only scratch
writable.

Every managed task, including the manager, also follows [the managed task protocol](task-protocol.md) for its
capability token, run paths, and task lifecycle.

## Delegation

A delegating task finishes only after all its descendants complete. When a skill requires another skill:

1. Call `edict_task_add(token, skill, title)` using a child permitted by `edict_registry`.
2. Prepare the complete token-free child instructions. The first line must be exactly `$<registered skill>`
   (for example, `$edict-signal-analysis`). Then give the absolute assigned `SKILL.md` path, the exact
   bounded source package, source checkout, scratch paths, and expected outcome. Resolve the skill path against the
   installed skills directory, the parent of your own skill directory. Do not include credentials or placeholders.
   Call `edict_delegate(token, taskId, prompt)` to store those instructions. Copy your capability token unchanged from
   the server response; never retype, shorten, normalize, or reconstruct it. A rejected capability is a lifecycle
   failure, not a reason to guess or edit the token before retrying.
   The server records the full assignment in its plan and logs. It returns a short `prompt` containing the child's
   skill declaration and credentials plus an instruction to fetch its assignment from `edict_task_get`.
3. Use native `spawn_agent` without inherited conversation (`fork_turns: "none"`, or `fork_context: false` in runtimes
   exposing that parameter). Use the returned short launch `prompt` as its `message`. Do not append task details or copy
   the stored assignment; the child obtains those instructions directly from MCP.
   Do not fork a conversation containing your parent or sibling  capabilities. Do not execute the child's skill inline or use an unmanaged copy as a fallback.
4.  Start independent siblings within the reserved capacity before waiting; only dependent work waits for a child.
   To wait, call `wait_agent` with `timeout_ms: 3600000`, and call it again the same way if it times out. Do not
   check on running children with `edict_plan_get` or `list_agents`.
   When a child completes or fails, read its recorded status and result with `edict_plan_get(taskId)`; without
   `taskId` it returns the whole plan, which you do not need for that.
   Respect the runtime's concurrency limit and reserve room for descendants. On failure stop dependent work;
   the root manager continues independent stages selected by its pipeline, including promotion after failed
   enrichment or generation. A managed worker must finish its own task as failed unless its skill explicitly permits
   tolerating that failure. If spawning
   fails or a worker is lost before finishing (it cannot fetch its assignment, its thread dies, or it stops responding),
   call `edict_task_cancel(token, taskId, result)` with your parent capability. That marks the task `cancelled`,
   revokes it and its descendants, and records your reason. Do not act as its worker. Retry it when your skill still
   needs its result: `edict_delegate` the same task again and launch a fresh worker. A cancelled task does not count
   against review limits and does not stop you from completing; a child that finished `failed` normally does. A
   coordinator may tolerate failed children only when its own managed skill defines an explicit threshold and the
   server accepts that coordinator's completed status.
