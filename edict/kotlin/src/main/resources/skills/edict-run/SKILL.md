---
name: edict-run
description: Execute the managed preparation, distribution, and generation pipeline as a delegated edict_manager worker.
---

# Edict Run

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-run`. Start your
assigned task. Coordinate only; every stage runs in its own native subagent.

1. Delegate `edict-prepare` with no write operations. Pass the source project, state context, and scratch root. It
   returns the bounded inbox snapshot and validates prerequisites.
2. Delegate `edict-distribution` with `inbox.delete`, `cluster.write`, and `cluster.signal.write`, scoped to
   selected inbox paths and `clusters`. Pass the prepared snapshot. Require every selected signal to have a verified
   durable destination before advancing.
3. Delegate `edict-generation` with `cluster.write`, `cluster.signal.write`, `example.write`, and
   `inspection.write`, scoped to resulting cluster directories and their exact inspection paths. Pass the returned
   affected cluster IDs and any pre-existing Pending clusters requested by the user. Scratch must be outside the state
   root.
4. Validate the complete frozen generation target set against the children's persisted outcomes. Read the resulting
   descriptions, histories, and inspection paths through MCP. Every Generated cluster must have its exact accepted
   inspection, completed reviews/measurements for that hash, and structurally valid assigned examples. Every Pending or
   Invalid cluster must preserve valid partial artifacts and record its concrete reason. Stop on a failed stage,
   missing target, or broken transition; do not repair a worker's artifacts inline. After successful validation,
   summarize produced inspections and every Pending/Invalid cluster with its history reason, then finish your task.

An empty inbox is a successful distribution no-op. Generation may still process explicitly selected or existing Pending
clusters. Valid Pending/Invalid domain outcomes do not fail the stage. Do not create worktrees, commit, push, or publish
repository changes: the managed task/state receipt replaces the standalone workflow's Git publication.
