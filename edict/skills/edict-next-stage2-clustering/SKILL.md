---
name: edict-next-stage2-clustering
description: Partition one Edict Next Stage-1 Signal group into the minimal set of distinct IntelliJ PSI inspection detectors. Use only for an Edict Next Stage-2 worker task with an input JSON, proposal path, and inspected Ultimate project.
---

# Edict Next Stage 2 Clustering

## Task contract

Accept exactly three paths from the prompt:

- `Stage-2 input JSON`: treat it as one self-contained Stage-1 group with `sourceClusterId`, `language`, and full Signal payloads.
- `Proposal output path`: the only file this task writes.
- `Inspected Ultimate project`: inspect the Signal revisions in this repository.

Read every Signal in the input. Do not inspect the Edict worktree, other Stage-1 inputs, other proposals, cluster history, existing inspections, or generation-task files. Do not edit either repository.

Write the proposal with exactly this shape:

```json
{"sourceClusterId":"stage1-id","groups":[{"signalIds":["signal-id"]}]}
```

Every input Signal ID must appear exactly once. Do not add cluster identity, description, ancestry, status, inspection metadata, reuse decisions, explanations, or other fields.

## Repository-context MCP contract

`mcp__qodana__file_at_ref`

- Input: the inspected Ultimate project and a Signal's `fileRevision.path` and `fileRevision.revision`, following the exposed tool schema.
- Output: the file contents at that revision, or an explicit lookup failure.
- Effect: read-only source retrieval; it does not change either repository.
- Next action: inspect the expected problem ranges when present. A lookup failure does not permit grouping by description alone; use the remaining source evidence and report the limitation in the task summary.

## Minimal structural partition

Start with the hypothesis that all input Signals belong to one group. Split only when a single IntelliJ inspection cannot classify the cases with one shared PSI traversal and one generalized detector predicate.

For every Signal, inspect its referenced code and ranges. Use source metadata such as the PR discussion, commit message, feedback reason, and embedded code snippet as evidence about intent. Ignore storage provenance.

Keep Signals together when one detector can cover them through the same:

- PSI element or visitor entry point;
- AST/PSI predicate;
- symbol, type, or call resolution checks;
- surrounding-context checks.

API names, wrapper functions, files, callers, feature areas, surface syntax, source kind, wording, and positive versus negative labels are not boundaries when the detector can represent them as alternatives of the same structural rule. A negative Signal is boundary evidence for the rule it constrains and stays with the corresponding positive rule when they are matching and non-matching forms of that detector.

Split only for materially incompatible PSI entry points, structural predicates, semantic-resolution requirements, or surrounding-context requirements that would constitute different inspections rather than variants handled by one inspection.

Before writing the proposal, compare the groups you intend to emit. Merge any pair that one generalized inspection visitor and detector predicate can handle. This is a local check within the supplied Stage-1 group; do not inspect or reconcile other Stage-1 groups.

A group may contain one Signal. There is no minimum size and no special singleton explanation. Cluster identity, ancestry, descriptions, and statuses are assigned later by cluster generation.
