---
name: managed-edict-distribution
description: Assign a prepared inbox snapshot to semantically coherent clusters through edict-mcp as a managed worker.
---

# Managed Edict Distribution

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-distribution`. This
leaf uses `inbox.delete`, `cluster.write`, and `cluster.signal.write` only.

Start the task and process the prepared inbox paths alphabetically. Read the complete stored signal and require its hash
to match the prepared snapshot. Preserve every field, including provenance and negative evidence. Infer source language
from actual recorded source, not a guessed cluster title.

For each plausible existing cluster, read its complete description and every member signal, including negatives. A
nearby description is only a navigation hint. Assign a signal only when it and every current member can be handled by
the same IntelliJ inspection and the languages match. Read exact-revision source through local Git or the inspection
server when source behavior is ambiguous. Otherwise select a new unused kebab-case cluster ID and a concise detector
description; do not split or rename existing clusters.

Use these ordered MCP writes:

1. Create a new `description.json` with matching `id`, `description`, `language`, and `status: "Pending"`, or preserve
   an existing description's other fields. When adding evidence to a Generated cluster, mark it Pending and preserve its
   current inspection as `predecessorId` so generation revalidates it.
2. Write the complete inbox content to `clusters/<cluster>/signals/<signal>.json`. If the destination already exists,
   require identical content; do not silently replace conflicting evidence.
3. Append the assignment and source signal ID to the cluster's `history.md` using its current hash. Read back the
   destination and verify its content/hash.
4. Only then delete the original inbox record with its original hash. If any step fails, stop, preserving the original
   inbox record whenever it still exists. Report partial progress so a retry can recognize an already copied identical
   signal.

This sequence preserves evidence across interruption; it is not a multi-file transaction. Never delete the only copy of
a signal. Return the exact assigned IDs, affected cluster IDs, and any remaining selected paths; finish only when every
selected signal has its verified destination.
