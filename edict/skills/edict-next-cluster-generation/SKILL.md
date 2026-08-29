---
name: edict-next-cluster-generation
description: Process one Edict Next cluster from evidence and reuse through candidate review and final decision.
---

# Edict Next Cluster Generation

The prompt supplies one cluster directory, one private scratch workspace, and the inspected project. Read and edit only
that cluster. Put transient state and worker output only in the scratch workspace. MCP owns terminal status and
inspection materialization; you may write only the cluster description/id before generation starts, each Signal's
`syntheticExampleId`, code examples, `candidate.inspection.kts`, and `history.md`. Do not change any other Signal
field.

Every Edict Next MCP response has an authoritative `nextAction`. Check `exhausted` first. When
`retryUnchanged: true`, the operation produced no verdict: repeat it with the exact same input. Finishing without a
finalization call is valid and makes repository validation infer predecessor fallback. Use `DISCONTINUED` only when
Signals conflict or no coherent PSI rule is feasible.

Keep `<private scratch workspace>/state.md` as the restart point. After every MCP call or worker result, record the
phase, cluster id and path, last `nextAction`, candidate path, sampled-findings path, review paths, and exact next step.
Re-read it after interruption or context compaction. Never put this file in the worktree.

## 1. Complete evidence

Read every Signal and inspect its exact source revision and ranges. Refine the description to name the shared detector.
Preserve the id when it still describes that detector; otherwise rename the cluster directory and the
`description.json` id together, using lowercase kebab-case. Do this before the inspection-action call, never after it.
Do not change membership, language, or `Pending` status.

For every Signal without `syntheticExampleId`, invoke `$edict-next-code-example`. Reuse a genuinely matching example
or create and MCP-validate one. Every Signal must end with a semantically correct, same-label example.

## 2. Decide reuse

Call `mcp__qodana__edict_next_get_inspection_action(clusterDirectory)`. This starts the configured cluster deadline,
which is 210 minutes by default.

- `CONFLICT`: append the opposite-label rationale and ids to history, then finalize `DISCONTINUED`.
- `SKIP`: append the reuse decision and finish without finalizing; repository validation keeps the predecessor.
- `GENERATE`: continue below.
- `exhausted: true`: append the deadline outcome and finish without finalizing.

A changed predecessor is skipped only when it still covers at least 85% of positives and reports no negative. An
unchanged terminal cluster is preserved without remeasurement.

## 3. Generate and measure

Write only Kotlin Inspection KTS to `candidate.inspection.kts`. The cluster id, description, and language are the rule
identity. Implement the general structural rule; never special-case example text, paths, names, or line numbers.

Call `mcp__qodana__edict_next_validate_inspection(clusterDirectory, inspectionPath)`. It compiles once and measures
every stored example. Acceptance requires at least one positive, no negative reports, and the returned positive-coverage
bar. A compiling coverage miss ratchets that bar from 85% to 70% to 50%.

- On `RETRY_SAME_INPUT`, call again with the candidate unchanged.
- On `REPAIR_INSPECTION`, fix the general predicate. Never weaken a correct negative example to pass.
- On `ANALYZE_PROJECT`, keep the exact validated candidate and continue.
- With no positive example or no feasible coherent rule, append the rationale and finalize `DISCONTINUED`.

## 4. Review project findings

Call `mcp__qodana__edict_next_get_new_inspection_results` for the validated candidate. It shares a batched project scan
with other clusters and writes up to 20 findings. Retry the exact input on `RETRY_SAME_INPUT`; stop without finalizing
when the deadline is exhausted.

Launch a fresh worker with:

```text
$edict-next-weak-signal-review

Sampled findings JSON: <returned sampledFindingsPath>
Review output path: <new JSON path in private scratch>
Cluster directory: <cluster directory>
Inspected project: <project path>
```

Validate that every finding index appears exactly once. On `INCOMPLETE`, launch a fresh worker while time remains or
finish without finalizing. When a `COMPLETE` review adds examples, repeat candidate validation and project analysis.

When a complete weak-signal review adds no examples, launch a fresh quality reviewer:

```text
$edict-next-inspection-review

Cluster directory: <cluster directory>
Candidate inspection: <candidate path>
Sampled findings JSON: <returned sampledFindingsPath>
Weak-signal review JSON: <completed review output>
Inspected project: <project path>
Review output path: <new JSON path in private scratch>
```

On `INCOMPLETE`, retry with a fresh reviewer or finish without finalizing. On `REVISE`, apply only evidence-backed
general corrections and repeat validation, project analysis, weak-signal review, and quality review for the revised
candidate. Only `ACCEPT` permits a generated decision.

## 5. Finish

Before finalization, append a concise history entry with the rule, reuse decision, attempts, failures, sampled-finding
review, quality review, achieved coverage, and decision. Then call
`mcp__qodana__edict_next_finalize_cluster(clusterDirectory, GENERATED, inspectionPath)` with the exact accepted
candidate. The call records its text; final repository validation remeasures and materializes it.

Every exit path must leave non-empty history explaining the outcome.
