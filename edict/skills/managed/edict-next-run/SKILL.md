---
name: managed-edict-next-run
description: Execute the managed preparation, distribution, and generation pipeline as a delegated edict_manager worker.
---

# Managed Edict Run

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-next-run`. Start your assigned task. Coordinate only; every stage runs in its own native subagent.

1. Delegate `edict-next-prepare` with no write operations. Pass the source project, state context, and scratch root. It returns the bounded inbox snapshot and validates prerequisites.
2. Delegate `edict-next-distribution` with `inbox.delete`, `cluster.write`, and `cluster.signal.write`, scoped to selected inbox paths and `clusters`. Pass the prepared snapshot. Require every selected signal to have a verified durable destination before advancing.
3. Delegate `edict-next-generation` with `cluster.write`, `cluster.signal.write`, `example.write`, and `inspection.write`, scoped to resulting cluster directories and their exact inspection paths. Pass the returned affected cluster IDs and any pre-existing Pending clusters requested by the user. Scratch must be outside the state root.
4. Verify the children's persisted task outcomes. Read the resulting descriptions and inspection paths through MCP; a Generated cluster must have its accepted inspection. Summarize produced inspections and all Pending/Invalid clusters with reasons from history. Finish your task.

An empty inbox is a successful distribution no-op. Generation may still process explicitly selected or existing Pending clusters. Do not create worktrees or publish repository changes.
