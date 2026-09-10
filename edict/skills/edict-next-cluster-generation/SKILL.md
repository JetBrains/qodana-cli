---
name: edict-next-cluster-generation
description: Process one Pending Edict Next cluster through evidence, reuse, candidate review, and a direct repository transition.
---

# Edict Next Cluster Generation

# Goal

Process the supplied Pending cluster from its Signals into one of these repository states:

- `Generated`: one IntelliJ inspection handles every Signal and has passed example validation and project review.
- `Discontinued`: the cluster Signals are semantically incompatible and do not express one coherent code-quality rule.
- `Invalid`: pipeline processing cannot continue because infrastructure/tooling failed or the cluster input/state is broken.
- `Pending`: work is incomplete, but every repository artifact left behind is structurally valid and can be continued later.

The prompt supplies `clusterId`, `clusterDirectory`, the absolute worktree path, an absolute private scratch directory, and the
inspected project.
Before the first write or MCP call, resolve the worktree and private scratch paths. Return failure without changing the repository
if the private scratch directory equals the worktree or is below it.

For every Qodana MCP call, pass the inspected IntelliJ project as `projectPath`. Never pass the Edict worktree as
`projectPath`; the worktree is repository data already loaded in the run context.

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

Do not load the `edict-next-code-example`, `edict-next-weak-signal-review`, `edict-next-inspection-code-review`, or
`edict-next-inspection-value-review` skills
yourself. Ask a fresh worker to load the required skill and use only its returned artifact.

The 120-minute cluster deadline starts with the first `edict_next_get_inspection_action` call and does not reset. Every
later cluster MCP call uses the remaining time. If an MCP call reports `Cleanup current session to valid Pending state and
stop generation`, stop child workers, leave the cluster and its artifacts in a structurally valid `Pending` state, and
return without another MCP call.

Use `Discontinued` if and only if exact Signal evidence proves that the Signals themselves have incompatible semantic
requirements and therefore cannot belong to one coherent code-quality rule. Record the incompatible Signal ids and the
semantic contradiction in history. Do not use `Discontinued` for implementation limits, missing or malformed evidence,
duplicate Signals or inspections, tool or infrastructure failures, timeouts, rejected candidates, or any other pipeline limitation.
Candidate failures, repeated poor decisions, and exhausted repair attempts do not by themselves prove `Invalid`. Keep repairing
while time remains; leave the cluster `Pending` when the deadline stops work. Use `Invalid` only when a concrete
infrastructure/tooling/capability failure or broken cluster input/state prevents further valid processing, and record that evidence.

# Process

## 1. Complete the evidence

Read `history.md` and prior attempt artifacts from private scratch before revising the cluster identity. Read every Signal and
retrieve its exact source revision and relevant ranges with `mcp__qodana__file_at_ref`. Use the current Signals and exact source
evidence as the authority for the cluster description and ID; history provides continuity but does not override current evidence.
Keep the current id when it fits; otherwise rename the cluster before the first MCP call. Record every ID or description change in
`history.md` with the old value, new value, reason, and supporting evidence. A rename changes the cluster directory name and
`description.json` id together. Preserve `predecessorId`: it identifies the existing inspection under its old id until the terminal
transition.

For every Signal without `syntheticExampleId`, start a fresh worker (create with **native** spawn_agent tool) with:

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

Generation constraints:

- Use only the Inspection KTS API and define one `localInspection { ... }` implementation in this file.
- Keep the complete inspection in this one file; add explicit imports only for symbols not provided by the Inspection KTS runtime.
- Do not hard-code repository paths, filenames, line numbers, or other example-specific details.
- Do not use data-flow analysis. If a semantically coherent rule requires it, apply the Invalid transition because the pipeline
  cannot implement the rule under its constraints.
- Keep traversal bounded and file-local. Reference searches may use only this exact form:

  ```kotlin
  val searchScope = LocalSearchScope(file)
  val references = ReferencesSearch.search(mainElement, searchScope).findAll()
  ```

  Do not use project-wide, module-wide, global, or other cross-file reference searches. If the rule requires such a search,
  apply the Invalid transition because the pipeline cannot implement the rule under its constraints.
- Prefer a semantically correct, realistically implementable inspection over a clever or brittle one.

Ask a fresh worker (create with **native** spawn_agent tool) to load `edict-next-inspection-code-review` before verification:

```text
Load the edict-next-inspection-code-review skill.

Cluster directory: <clusterDirectory>
Candidate inspection: <clusterDirectory>/../../inspections/<clusterId>.candidate.kts
Inspected IntelliJ project: <inspected project path>
Review output path: <privateScratchDirectory>/inspection-code-review.json
```

Read the review output. On `REJECT`, make the smallest suggested general correction and repeat the code review. If the
review identifies a duplicate existing inspection or another specific cluster problem that cannot be fixed, apply the
Invalid transition. Apply the Discontinued transition only if the review identifies semantically incompatible Signals that
cannot express one coherent code-quality rule.

Only after code review is accepted, call `edict_next_validate_inspection(clusterId)`. Acceptance requires at least one positive
example and 85% aggregate label accuracy.

- On `REPAIR_INSPECTION`, repair the general predicate and validate again.
- On `ANALYZE_PROJECT`, keep the exact validated candidate and continue.
- If exact Signal evidence proves that the Signals are semantically incompatible, record the incompatible Signal ids and
  contradiction, then apply the Discontinued transition. If the Signals express a coherent rule, continue repairing the candidate.
  Apply the Invalid transition only when a concrete pipeline capability or tooling failure blocks further valid processing.

## 4. Review project findings

Call `edict_next_get_new_inspection_results(clusterId, privateScratchDirectory)` and wait up to 40m. Then ask a fresh worker (create with **native** spawn_agent tool)
to load `edict-next-weak-signal-review`:

```text
Load the edict-next-weak-signal-review skill.

Review config: <weak-signal-review-config path returned by the MCP>
```

Read the returned summary. If it lists false-positive reports, read every report, repair the candidate's general predicate,
validate it, call the MCP again for a fresh pair of manifests, and repeat this step after validation returns `ANALYZE_PROJECT`.
If it lists no false positives, ask a fresh worker (create with **native** spawn_agent tool) to load `edict-next-inspection-value-review`:

```text
Load the edict-next-inspection-value-review skill.

Review config: <inspection-review-config path returned by the MCP>
```

- On value-review `REJECT`, read the findings. Apply the Invalid transition for a duplicate existing inspection or another
  broken cluster state you cannot fix. Apply the Discontinued transition only if exact Signal evidence proves that the Signals
  are semantically incompatible and cannot express one coherent code-quality rule. Otherwise, make the smallest suggested
  general corrections, then repeat validation and both reviews. Repeated rejection does not justify `Invalid`; leave the cluster
  `Pending` if the deadline stops further repair.
- On value-review `ACCEPT`, record the rule, attempts, reviews, achieved accuracy, and decision in history, then apply the
  Accepted transition.

## 5. Apply the terminal transition

- **Accepted:** replace `inspections/<clusterId>.inspection.kts` with the exact accepted candidate; remove the candidate
  and any distinct predecessor inspection; clear `predecessorId`; set the status to `Generated`.
- **Reused:** move the predecessor inspection to `inspections/<clusterId>.inspection.kts` when the id changed; remove the
  candidate; clear `predecessorId`; set the status to `Generated`.
- **Discontinued:** append the incompatible Signal ids and their semantic contradiction to history; remove the candidate and
  predecessor/current inspection; clear `predecessorId`; set the status to `Discontinued`.
- **Invalid:** append the concrete infrastructure/tooling failure or broken cluster input/state to history and set the status
  to `Invalid`. Keep valid partial artifacts and `predecessorId` unchanged, as for `Pending`.

Return after the repository reaches the chosen state.
