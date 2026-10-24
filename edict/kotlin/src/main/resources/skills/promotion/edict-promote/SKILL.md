---
name: edict-promote
description: Managed worker that promotes eligible generated inspections through configured pull requests.
---

# Inspection promotion

Run only as a delegated managed subagent.
Follow [the managed task protocol](../edict_manager/references/task-protocol.md).
Registry ID: `edict-promote`.

Call `edict_promote_clusters` with only your token by default. This promotes every `Generated` cluster having at least
five `STRONG` Signals with usable HTTP(S) source PR or commit diff URLs whose current inspection has not already been
promoted to the configured target. Generated evidence, feedback, weak Signals, and commit Signals without URLs do not
count toward that reference requirement. The server builds the PR description from the cluster's evidence, including
up to ten strong Signals with explanations, labels, source links, and exact file revisions and ranges. Missing references
are reported as a per-cluster failure before creating a PR; never invent URLs to bypass this check. When the task
explicitly names cluster IDs, pass exactly those IDs as `clusterIds`; the same eligibility rules still apply.

Repository identity, target branch, reviewer, and target inspections directory come from the server's `qodana.yaml`
configuration and must never be overridden in a tool call. The operation does not promote the same inspection digest
to the same target twice.

Review every per-cluster failure and report created and failed clusters. Provider credentials remain in
the MCP server environment; never request, print, or persist them.
