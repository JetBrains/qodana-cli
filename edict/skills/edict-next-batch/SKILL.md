---
name: edict-next-batch
description: Assign exactly one prepared Edict inbox batch to durable clusters; validation belongs to its orchestration agent.
---

# Edict Next Batch

Load only this skill. The task supplies `batchPath`, the Edict worktree, and the inspected project. Process exactly this
batch and edit the worktree directly. Do not call batch validation; the distribution orchestrator owns it.

## Batch file

`batchPath` contains `currentBatchInboxSignals`. Each entry has the real inbox `signalPath` and nearest candidates:

- `cluster`: current `clusterId`, `clusterDescription`, real `clusterPath`, and `nearestDistance`.
- `signal`: another selected inbox `signalId`, its real `signalPath`, and `nearestDistance`.

Candidates are deterministic help, not decisions. Read each Signal, the relevant cluster members, and source at the
referenced revision. Do not group from description distance alone.

## Workflow

1. Decide one owner for every `currentBatchInboxSignals` entry. Assign it to an existing cluster only when one rule can
   cover it through the same PSI entry point and structural predicate. A negative Signal belongs with the positive rule
   it constrains.
2. Group remaining batch Signals into coherent new clusters. A singleton is valid. Use a lowercase kebab-case id and a
   detector-focused description. API names, files, callers, syntax, wording, source kind, and label are not boundaries
   when one structural rule can cover them.
3. Move each Signal JSON from `inbox/` to exactly one `<cluster>/signals/` directory without changing its content.
   For a new cluster, create only:

   ```text
   clusters/<id>/description.json  id, description, language, status Pending; no inspection metadata
   clusters/<id>/history.md        non-empty creation note
   clusters/<id>/signals/*.json    moved current-batch Signals
   ```

   Existing clusters receive the moved Signal files and must have their status changed to `Pending`. Do not change
   their id, description, language, history, examples, outcome, candidate, or inspection during routing. Do not touch
   Signals outside this batch.
4. Return a concise completion report to the orchestrator. Do not validate, repair after validation, prepare another
   batch, add or run tests, or generate inspections.

For source context, use read-only repository MCPs such as `mcp__qodana__file_at_ref` with the Signal's exact revision.
