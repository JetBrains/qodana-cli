---
name: edict-promote
description: Managed worker that promotes eligible generated inspections through configured pull requests.
---

# Inspection promotion

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md).
Registry ID: `edict-promote`.

Call `edict_promote_clusters` with only your token by default. This promotes every `Generated` cluster having more than
4 `STRONG` Signals whose current inspection has not already been promoted to the configured target. When the task
explicitly names cluster IDs, pass exactly those IDs as `clusterIds`; the same eligibility rules still apply.

Repository identity, target branch, reviewer, and target inspections directory come from the server's `qodana.yaml`
configuration and must never be overridden in a tool call. The operation does not promote the same inspection digest
to the same target twice.

Review every per-cluster failure and report created and failed clusters. Provider credentials remain in
the MCP server environment; never request, print, or persist them.
