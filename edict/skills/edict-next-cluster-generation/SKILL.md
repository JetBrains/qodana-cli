---
name: edict-next-cluster-generation
description: Materialize and process exactly one validated Edict Next Signal group by assigning cluster metadata, completing evidence, deciding reuse, generating and validating an inspection, and recording the terminal outcome.
---

# Edict Next Cluster Generation

The task supplies one generation-task JSON, the evidence catalog, the Edict worktree, and the inspected project. The task JSON identifies exactly one group from the validated Stage-2 partition. Do not read another task or edit another final cluster.

## Materialize cluster metadata

1. Read every Signal in the group from the evidence catalog and inspect every referenced source revision and relevant range. Infer the single AST/PSI detector represented by the already-validated group.
2. Choose the final cluster ID and a precise description of that detector now. Stage 2 deliberately did not choose them. Reuse a predecessor ID and description only when they still describe the complete group accurately; otherwise choose a lowercase kebab-case ID from the rule. If the directory already exists, append a short deterministic Signal-set suffix so concurrent tasks cannot overwrite each other; the root agent may normalize IDs later.
3. Set `ancestorIds` to exactly the original cluster IDs owning any Signal in this group, as recorded in the evidence catalog. This lineage is assigned after semantic partitioning and must not affect the partition.
4. Create `clusters/<id>/description.json` with the chosen ID, description, language, exact ancestry, and status `Pending`. Copy every complete Signal JSON into `signals/`. Move inbox Signals semantically by copying them from the catalog; never change a Signal field except later assignment of a missing `syntheticExampleId`.
5. Preserve every example from every ancestor, including all ancestor examples when an old cluster was split. Preserve an unchanged predecessor history; otherwise combine predecessor histories with source headings and append the materialization decision. Do not carry inspection metadata into `description.json` yet.

After this section, use the newly created absolute cluster directory for every MCP call below.

## MCP contracts

`mcp__qodana__edict_next_get_inspection_action`

- Input: the absolute `clusterDirectory`.
- Output: `action` (`DEDUPLICATE`, `CONFLICT`, `SKIP`, or `GENERATE`), `summary`, optional `droppedSignalIds`, `conflictingSignalIds`, `statusToKeep`, `previousRuleId`, and `previousInspectionPath`, plus `exhausted`.
- Effect: first resolves same-code duplicates involving at least one inbox Signal, then performs reuse evaluation against the pre-clustering snapshot. Signal identity is the complete `fileRevision`: path, revision, and ranges; descriptions do not participate. For a changed one-to-one predecessor, the old inspection is reusable under the same 85% total acceptance threshold as candidate validation. The first call starts this cluster's monotonic 90-minute deadline; repeated calls do not reset it. Returned drops are recorded in run state, but the MCP call does not edit the worktree. The decision and reason are reported to stdout.
- Next action: check `exhausted` first. For `DEDUPLICATE`, apply exactly the returned removals and call the operation again. For `CONFLICT`, discontinue only this cluster. For `SKIP`, apply `statusToKeep` and preserve the returned predecessor artifacts as applicable. For `GENERATE`, proceed with rule and code generation.

`mcp__qodana__edict_next_validate_inspection`

- Input: absolute `clusterDirectory` and absolute candidate `inspectionPath` inside the worktree. Call the action operation first.
- Output: `compilationSuccess`, `overallSuccess`, `summary`, per-example `failures` containing `exampleId` and `message`, and `exhausted`.
- Effect: read-only compilation and execution against every stored mandatory example. `overallSuccess` is true when at least 85% of all examples pass. `failures` still contains every failed example. The result is reported to stdout.
- Next action: check `exhausted` first. Revise and retry unless `overallSuccess` is true; only then request project results.

`mcp__qodana__edict_next_get_new_inspection_results`

- Input: absolute `clusterDirectory` and the validated candidate `inspectionPath`.
- Output: `success`, `sampledResults`, absolute `sampledFindingsPath`, `summary`, and `exhausted`.
- Effect: runs the candidate on the inspected project in a shared inspection batch, then writes this inspection's sampled findings to its own JSON file and reports the result to stdout. It does not classify findings or modify the worktree. Allow at least 25 minutes for this MCP call: it may first wait for a previous batched analysis to finish and then run the new analysis, and either phase can easily take 10 minutes.
- Next action: check `exhausted` and `success` first. Launch a fresh weak-signal-review task with the returned findings file. Kotlin/MCP does not decide whether generation can finish.

## Complete evidence, deduplicate, and decide reuse

For every Signal whose `syntheticExampleId` is absent, invoke `$edict-next-code-example`. Preserve all accumulated examples; every example is mandatory evidence. Then call `mcp__qodana__edict_next_get_inspection_action`.

- For `DEDUPLICATE`, remove exactly `droppedSignalIds` from this cluster's `signals/`. Remove newly created examples referenced only by dropped Signals, but preserve every inherited example. Recompute `ancestorIds` from the retained Signals and `ownership.json`. If the retained cluster now exactly matches one predecessor, restore that predecessor's complete cluster directory byte-for-byte under its original ID; otherwise append the deterministic removals to `history.md`. Call the operation again with the resulting cluster directory. Do not compare descriptions when deciding duplicates. The service retains an already-clustered Signal when one exists; otherwise it retains the lexicographically smallest Signal ID.
- For `CONFLICT`, retain all Signals and their completed evidence, set status `Discontinued`, clear inspection metadata, remove candidate code, append the conflicting Signal IDs and opposite-label rationale to `history.md`, and finish this cluster successfully. Do not fail or stop unrelated cluster tasks.
- Repeat only for `DEDUPLICATE`; then handle `SKIP` or `GENERATE` below.

- For `SKIP`, update `description.json` to `statusToKeep`. Preserve or restore the returned prior rule ID and inspection for `Generated`; preserve `Discontinued` or `NotGenerated` exactly when returned. Remove `candidate.inspection.kts`, append the reuse decision to `history.md`, and finish.
- For an exhausted response, set status `NotGenerated`, remove candidate code and inspection metadata, append the deadline outcome to `history.md`, and finish.
- For `GENERATE`, reconsider the semantic rule. Reuse the predecessor rule and rule ID if it is still applicable; otherwise choose new metadata. Rule-ID collision handling is deferred to the root agent.
- If the Signals are contradictory or no coherent PSI rule is feasible, set status `Discontinued`, clear inspection metadata, remove candidate code, append the rationale to `history.md`, and finish.

## Generate and validate

1. Put complete inspection metadata in `description.json`: `ruleId`, `ruleName`, `severity`, and `inspectionDescription`. Write Kotlin Inspection KTS code to `candidate.inspection.kts`. Use `mcp__qodana__generate_inspection_kts_api`, `mcp__qodana__generate_inspection_kts_examples`, and PSI-tree tools when needed. Implement the general rule; never special-case examples, paths, names, or line numbers.
2. Call `mcp__qodana__edict_next_validate_inspection` with the candidate path. If `overallSuccess` is false, revise the rule or code and retry. Do not request project results before the validation threshold passes. Retain the returned failures as evidence even when `overallSuccess` is true.
3. Call `mcp__qodana__edict_next_get_new_inspection_results`. Launch one fresh task with this exact shape:

   ```text
   $edict-next-weak-signal-review

   Sampled findings JSON: <absolute sampledFindingsPath returned by MCP>
   Review output path: <absolute new JSON path beside the sampled findings file>
   Cluster directory: <absolute cluster directory>
   Inspected Ultimate project: <absolute project path>
   ```

   Read and validate the worker output. If its status is `INCOMPLETE`, do not finalize the inspection; launch a fresh review worker while the deadline permits, or finish as `NotGenerated` when the evidence cannot be completed. If it is `COMPLETE` and lists created examples, repeat inspection validation and project analysis. Every created example is mandatory evidence.
4. When validation passes and a `COMPLETE` weak-signal review creates no new examples, launch one fresh task with this exact shape:

   ```text
   $edict-next-inspection-review

   Cluster directory: <absolute cluster directory>
   Candidate inspection: <absolute candidate inspection path>
   Sampled findings JSON: <absolute sampled findings path returned by MCP>
   Weak-signal review JSON: <absolute completed review output path>
   Inspected Ultimate project: <absolute project path>
   Review output path: <absolute new JSON path beside the weak-signal review>
   ```

   Read and validate the reviewer output. For `INCOMPLETE`, launch a fresh reviewer while the deadline permits or finish as `NotGenerated` when the evidence cannot be completed. For `REVISE`, apply only evidence-backed general corrections, then repeat inspection validation, project analysis, weak-signal review, and inspection review for the exact revised candidate. Do not finalize from a review of older code.
5. When the fresh inspection review returns `ACCEPT`, move the exact reviewed candidate to `inspections/<ruleId>.inspection.kts`, set status `Generated`, and append a concise rule, reuse, attempts, failures, sampled-finding review, inspection-quality review, and final decision record to `history.md`.
6. If the service reports exhaustion at any point, set status `NotGenerated`, remove candidate code and inspection metadata, append the outcome to `history.md`, and finish.

A terminal cluster must have no `candidate.inspection.kts` and must have a non-empty `history.md`.
