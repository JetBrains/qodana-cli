---
name: edict-next-batch
description: Assign one prepared alphabetical batch of Edict inbox Signals to existing or new durable clusters, then validate the worktree transition.
---

# Edict Next Batch

The task supplies `batchPath`, the Edict worktree, and the inspected project. Process exactly this batch. Edit the
worktree directly; MCP validates the result but does not apply it.

## Batch file

`batchPath` contains `currentBatchInboxSignals`. Each entry has the real inbox `signalPath` and nearest candidates:

- `cluster`: `clusterId`, `clusterDescription`, real `clusterPath`, `closestSignalIds`, and `nearestDistance`.
- `signal`: another current-batch `signalId`, its real `signalPath`, and `nearestDistance`.

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
4. Call `mcp__qodana__edict_next_validate_batch` with the exact `batchPath`.
5. Follow `nextAction`: on `REPAIR_BATCH`, repair only the reported transition issues and retry the same path; on
    `PROCESS_BATCH`, report the returned next `batchPath` to the orchestration task; on `START_GENERATION`, report that
    the inbox is empty. Retrieval failures are retried internally; report and stop if the tool still fails.

The validation response contains `success`, `validatedSignalCount`, `inboxSignalCount`, optional next `batchPath`,
`summary`, `issues`, and `nextAction`. A successful call prepares the next batch before replacing the rolling verified
snapshot. Do not add or run tests and do not generate inspections in this skill.

For source context, use read-only repository MCPs such as `mcp__qodana__file_at_ref` with the Signal's exact revision.
