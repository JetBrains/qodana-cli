---
name: edict-next-cluster-generation
description: Process one Pending Edict Next cluster through evidence, reuse, candidate review, and a direct repository transition.
---

# Edict Next Cluster Generation

# Goal

Process the supplied Pending cluster from its Signals into one of these repository states:

- `Generated`: one IntelliJ inspection handles every Signal and has passed example validation and project review.
- `Discontinued`: sound cluster evidence shows that no coherent IntelliJ inspection can fit signal requirements.
- `Invalid`: conflicting evidence or another specific cluster problem requires manual repair.
- `Pending`: work is incomplete, but every repository artifact left behind is structurally valid and can be continued later.

The prompt supplies `clusterId`, `clusterDirectory`, a private scratch directory, and the inspected project.

# Allowed changes

You may directly change only:

- the target cluster directory name and the `id`, `description`, `status`, and `predecessorId` fields in its
  `description.json`; a rename must change the directory name and `id` together;
- appended entries in the target cluster's `history.md`;
- `inspections/<clusterId>.candidate.kts`, `inspections/<clusterId>.inspection.kts`, and the inspection named by the
  cluster's `predecessorId`;
- files in the supplied private scratch directory.

Do not directly change anything else. Do not edit signals, change cluster membership or language, edit the inspected project, or edit
another cluster. Put all transient worker output in private scratch directory.

Keep the cluster `Pending` and preserve `predecessorId` until a terminal transition. After renaming the cluster, use its
new id in every MCP call and inspection path; also rename an existing candidate to the new candidate path.

Do not load the `edict-next-code-example`, `edict-next-weak-signal-review`, or `edict-next-inspection-review` skills
yourself. Ask a fresh worker to load the required skill and use only its returned artifact.

# Process

## 1. Complete the evidence

Read `history.md` and prior attempt artifacts from private scratch before revising the cluster identity. Read every Signal and
retrieve its exact source revision and relevant ranges with `mcp__qodana__file_at_ref`. Use the current Signals and exact source
evidence as the authority for the cluster description and ID; history provides continuity but does not override current evidence.
Keep the current id when it fits; otherwise rename the cluster before the first MCP call. Record every ID or description change in
`history.md` with the old value, new value, reason, and supporting evidence. A rename changes the cluster directory name and
`description.json` id together. Preserve `predecessorId`: it identifies the existing inspection under its old id until the terminal
transition.

For every Signal without `syntheticExampleId`, start a fresh worker with:

```plaintext
Load the <edict-next-code-example invocation call> skill.

Signal path: <clusterDirectory>/signals/<signal-id>.json
Synthetic examples directory: <clusterDirectory>/synthetic-examples
```

Verify the signal now has the `syntheticExampleId` field. If the worker cannot complete the assignment, save the reason and follow steps
to apply `Invalid` transition below.

## 2. Decide whether to reuse the predecessor

Call `edict_next_get_inspection_action(clusterId)`.

- `CONFLICT`: record the conflicting Signal ids and rationale in history, then apply the Invalid transition below.
- `SKIP`: record the measured reuse decision in history, then apply the Reused transition below.
- `GENERATE`: continue with a candidate.

## 3. Generate and measure a candidate

Before writing the first candidate, call `mcp__qodana__generate_inspection_kts_api` and
`mcp__qodana__generate_inspection_kts_examples` for the cluster language. Call `mcp__qodana__generate_psi_tree` on
representative positive and negative code examples whenever the relevant PSI structure is uncertain.

Write the candidate to `inspections/<clusterId>.candidate.kts`. Implement one general IntelliJ inspection for the shared
problem. Never special-case example text, paths, names, or line numbers.

Call `edict_next_validate_inspection(clusterId)`. Acceptance requires at least one positive example and 85% aggregate
label accuracy.

- On `REPAIR_INSPECTION`, repair the general predicate and validate again.
- On `ANALYZE_PROJECT`, keep the exact validated candidate and continue.
- If sound cluster evidence proves that no coherent PSI rule is feasible, record why and apply the Discontinued
  transition. If a specific problem with the cluster itself prevents that decision, apply the Invalid transition instead.

## 4. Review project findings

Call `edict_next_get_new_inspection_results(clusterId, privateScratchDirectory)` and wait up to 40m. Then ask a fresh worker to
load `edict-next-weak-signal-review`:

```text
Load the edict-next-weak-signal-review skill.

Review config: <weak-signal-review-config path returned by the MCP>
```

Read the returned summary. If it lists false-positive reports, read every report, repair the candidate's general predicate,
validate it, call the MCP again for a fresh pair of manifests, and repeat this step after validation returns `ANALYZE_PROJECT`.
If it lists no false positives, ask a fresh worker to load `edict-next-inspection-review`:

```text
Load the edict-next-inspection-review skill.

Review config: <inspection-review-config path returned by the MCP>
```

- On `REJECT`, read the findings. Apply the Invalid transition for a duplicate existing inspection or another specific
  cluster problem you cannot fix. Apply the Discontinued transition if the evidence proves that no coherent inspection
  can satisfy the Signals. Otherwise, make the smallest suggested general corrections, then repeat validation and both reviews.
- On `ACCEPT`, record the rule, attempts, reviews, achieved accuracy, and decision in history, then apply the Accepted
  transition.

## 5. Apply the terminal transition

- **Accepted:** replace `inspections/<clusterId>.inspection.kts` with the exact accepted candidate; remove the candidate
  and any distinct predecessor inspection; clear `predecessorId`; set the status to `Generated`.
- **Reused:** move the predecessor inspection to `inspections/<clusterId>.inspection.kts` when the id changed; remove the
  candidate; clear `predecessorId`; set the status to `Generated`.
- **Discontinued:** remove the candidate and predecessor/current inspection; clear `predecessorId`; set the status to
  `Discontinued`.
- **Invalid:** append the specific manual-repair reason to history and set the status to `Invalid`. Keep valid partial
  artifacts and `predecessorId` unchanged, as for `Pending`.

Return after the repository reaches the chosen state.
