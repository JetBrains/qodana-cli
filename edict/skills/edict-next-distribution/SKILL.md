---
name: edict-next-distribution
description: Sequentially assign prepared inbox Signals to durable clusters.
---

# Edict Next Distribution

Load only this skill. Do not read another Edict Next skill or edit the worktree directly. The stage budget is 120 minutes.

## Repository format

For the `signalId` returned by `edict_next_next_signal`, read the Signal from:

```text
<worktree>/inbox/<signal-id>.json
```

Here, a "neighbor" only means one of the smallest embedding distances in the retrieval corpus. It is a candidate for
comparison, not a guarantee of semantic closeness or cluster compatibility. For a neighboring cluster returned by the
MCP, read its description and the neighboring Signals assigned to it from:

```text
<worktree>/clusters/<cluster-id>/description.json
<worktree>/clusters/<cluster-id>/signals/<signal-id>.json
```

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

Treat the cluster description only as a navigation hint: it is derived from its Signals and may be close without fitting
the new Signal exactly. Decide from the Signal payloads by asking: **Can the received Signal and every Signal already in
this cluster be detected by the same IntelliJ inspection?** Assign it only when the answer is yes and the source language
matches. Positive and negative labels may belong to the same cluster when they define the behavior of that same
inspection. Otherwise, create a new cluster whose id and description express the inspection that should cover the polled signal.

Repeat until `edict_next_next_signal` returns `STOP_DISTRIBUTION`:

1. Call `edict_next_next_signal`.
2. Choose a compatible existing cluster id, or choose a new kebab-case id and concise description.
3. Call `edict_next_add_signal_to_cluster` with `signalId`, `clusterId`, and `newClusterDescription` only for a new id.

Signals are returned alphabetically and preparation selects at most 100. Return only after distribution is complete.
