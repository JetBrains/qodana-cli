---
name: edict-next-distribution
description: Sequentially assign prepared inbox Signals to durable clusters.
---

# Edict Next Distribution

Load only this skill. Do not read another Edict Next skill or edit the worktree directly. The stage budget is 120 minutes.

For every Qodana MCP call, pass the inspected IntelliJ project as `projectPath`. Never pass the Edict worktree as
`projectPath`; the worktree is repository data already loaded in the run context.

## Repository format

`edict_next_next_signal` returns the complete stored JSON for the incoming Signal. Preserve it as the authority for the
distribution decision; do not replace it with a summary.

Here, a "neighbor" only means one of the smallest embedding distances in the retrieval corpus. It is a candidate for
comparison, not a guarantee of semantic closeness or cluster compatibility. Before considering an existing cluster, call
`edict_next_get_distribution_context` with `kind: "cluster"` and its id. It returns the complete stored cluster description
and every member Signal JSON, including negative Signals. Loading only the description or nearest member is insufficient.

For a neighboring inbox Signal, call `edict_next_get_distribution_context` with `kind: "signal"` and its id when that
comparison is useful. It returns the complete stored Signal JSON.

`description.json` has this relevant shape:

```json
{
  "id": "cluster-id",
  "description": "general detector description",
  "language": "Kotlin",
}
```

A Signal JSON has this relevant shape:

```json
{
  "id": "signal-id",
  "fileRevision": {
    "path": "src/Foo.kt",
    "revision": "commit revision",
    "expectedRanges": [{"start": 12, "end": 12}]
  },
  "label": "POSITIVE",
  "description": "problem demonstrated by this Signal"
}
```

Only decision-relevant fields are shown; both files contain additional fields.

The stored evidence should usually be sufficient. When its revision, diff, or expected range leaves source behavior
ambiguous, inspect the repository at the exact recorded revision, for example with `get_file_at_ref`. Repository reads are
read-only evidence gathering; distribution mutations must still go only through the Edict Next MCP tools.

Treat the cluster description only as a navigation hint: it is derived from its Signals and may be close without fitting
the new Signal exactly. Decide from the Signal payloads by asking: **Can the received Signal and every Signal already in
this cluster be detected by the same IntelliJ inspection?** Assign it only when the answer is yes and the source language
matches. Positive and negative labels may belong to the same cluster when they define the behavior of that same
inspection. Otherwise, create a new cluster whose id and description express the inspection that should cover the polled signal.

Repeat until `edict_next_next_signal` returns `STOP_DISTRIBUTION`:

1. Call `edict_next_next_signal`.
2. Read the returned incoming Signal JSON. For each plausible existing cluster, call
   `edict_next_get_distribution_context(kind: "cluster", id: "<cluster-id>")` and compare the incoming Signal against every
   returned member Signal. Retrieve neighboring Signal context or exact-revision source only when it resolves a real ambiguity.
3. Choose a compatible existing cluster id, or choose a new kebab-case id and concise description. Do not split an existing
   cluster; cluster splitting is a manual repair outside this stage.
4. Call `edict_next_add_signal_to_cluster` with `signalId`, `clusterId`, and `newClusterDescription` only for a new id. The MCP
   rejects assignment to an existing cluster unless that cluster's context was successfully returned for the current Signal.
   If it returns `added: false`, treat `summary` as decision feedback, correct the cluster choice, and retry the same
   Signal. Do not call `edict_next_next_signal` while the Signal remains unassigned. Return a failed stage only when the
   response does not describe a correctable distribution decision or the corrected attempt also fails.

Signals are returned alphabetically and preparation selects at most 100. Return only after distribution is complete.
