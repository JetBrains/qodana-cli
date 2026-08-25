---
name: edict-next-clustering
description: Cluster every existing and inbox Edict Signal into a lineage-blind semantic partition and validate it before cluster metadata or repository state is changed.
---

# Edict Next Clustering

## MCP contracts

`mcp__qodana__edict_next_prepare_clustering`

- Input: the absolute `worktreePath`.
- Output: absolute `manifestPath` and `outputDirectory`, plus `signalCount` and the previous `clusterCount`.
- Effect: snapshots the complete clustered and pending Signal state as the validation baseline and writes the Stage-1 manifest in the agent workspace. It does not edit the worktree.
- Next action: run the bundled clustering script with the returned paths. Treat either count as diagnostic; do not use it instead of reading the manifest.

`mcp__qodana__edict_next_validate_cluster_view`

- Input: the absolute `partitionPath` inside the agent workspace.
- Output: `success`, `summary`, and zero or more `issues`, each with `path` and `message`.
- Effect: read-only validation that the proposed groups contain every snapshotted Signal exactly once, contain no unknown Signal, are non-empty, and do not mix languages. A successful partition is remembered for reuse decisions and final repository validation.
- Next action: continue to cluster generation only when `success` is true. On false, repair only the declarative partition and retry; do not edit the worktree.

## Stage 1

1. Call `mcp__qodana__edict_next_prepare_clustering` with the absolute worktree path. This snapshots all clustered Signals and JVM Signals from `inbox/`. Non-JVM inbox Signals are ignored and remain queued.
2. In one guarded shell invocation, create a virtual environment outside the agent workspace with `VENV_DIR=$(mktemp -d "${TMPDIR:-/tmp}/edict-next-clustering.XXXXXX")`, immediately install `trap 'rm -rf "$VENV_DIR"' EXIT`, create the environment with `python3 -m venv "$VENV_DIR"`, and install `scripts/requirements.txt` from this skill with `"$VENV_DIR/bin/pip"`. Do not split creation, installation, use, and cleanup across shell invocations.
3. In that same guarded invocation, run `scripts/cluster.py --manifest <manifestPath> --output <outputDirectory>` with `"$VENV_DIR/bin/python"`. Do not change the model, revision, distance threshold, size penalty, or language partition from the manifest.

## Stage 2 semantic partition

Before launching tasks, resolve every Stage-1 Signal ID to its full Signal payload. Write one self-contained input file per Stage-1 cluster under the agent workspace. Include `id`, `fileRevision`, `source`, `label`, `description`, derived `language`, and `strength`. Do not include the Signal's storage path, `syntheticExampleId`, original cluster ownership, cluster description or status, history, inspection metadata, or whether it came from `inbox/`.

For every Stage-1 input file, launch one logical subagent task, with no more than eight running concurrently. Use this exact task shape so every worker explicitly loads the dedicated Stage-2 contract instead of receiving a paraphrase:

```text
$edict-next-stage2-clustering

Stage-2 input JSON: <absolute input path>
Proposal output path: <absolute output path>
Inspected Ultimate project: <absolute project path>
```

Do not replace the skill invocation with custom grouping instructions. Give the worker no Edict worktree path, evidence-catalog path, other Stage-1 inputs, or other proposal files.

## Combine and validate

The root agent validates each proposal locally against its Stage-1 input, then combines all groups into:

```json
{"groups":[{"signalIds":["signal-id"]}]}
```

Call `mcp__qodana__edict_next_validate_cluster_view` with this combined partition. If it fails, repair the proposal files and retry. Do not materialize clusters, assign metadata, or change the worktree in this skill.

After validation, build a lossless evidence catalog under the workspace for the generation tasks:

```text
evidence-catalog/
  signals/<signal-id>.json
  ownership.json
  clusters/<original-cluster-id>/description.json
  clusters/<original-cluster-id>/history.md
  clusters/<original-cluster-id>/synthetic-examples/...
  inspections/...
generation-tasks/<group-index>.json
```

`ownership.json` maps every Signal ID to its original cluster ID or `null` for an inbox Signal. Copy files byte-for-byte; do not normalize Signal or example contents. Each generation-task file has this contract:

```json
{"signalIds":["signal-id"],"evidenceCatalogPath":"/absolute/path/to/evidence-catalog"}
```

The clustering workers must never see the evidence catalog or generation-task files; they exist solely for the later per-cluster generation phase.
