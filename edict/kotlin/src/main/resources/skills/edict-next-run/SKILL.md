---
name: edict-next-run
description: Managed coordinator subagent that orchestrates the Edict Next distribution and generation stages.
---

# Edict Next Run

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use it for every child delegation and task transition.

Load only this skill. Launch every stage with the native `spawn_agent` tool and mention its skill only in the first line
of the fresh worker prompt. Do not edit the state repository, call cluster-processing MCPs, or run tests yourself.

Call `edict_context` to verify the configured run context. Every delegated stage must obtain its paths from
`edict_context`; do not copy the state path into its prompt.

Run sequentially:

1. Launch `edict-next-distribution` for up to 120 minutes. Distribution prepares the pipeline snapshot before assigning
   Signals. After it returns, call
   `edict_next_validate_distribution` and stop on failure.
2. Launch `edict-next-generation` for up to 660 minutes. It obtains the configured state, project, and scratch paths
   directly from `edict_context`.
3. Call the read-only `edict_next_validate_generation` for up to 40 minutes. Stop unless it returns `PUBLISH`.

After successful validation, collect every Invalid cluster id and its recorded infrastructure/tooling or cluster-state failure
from `history.md`, then let the Qodana script finish. Pending and Invalid clusters do not block
publication. In the final response, list every Invalid cluster and its recorded reason. Wait for workers and MCP calls in
chunks of at most 60 minutes and stop on failure or timeout. The complete session budget is 900 minutes.
