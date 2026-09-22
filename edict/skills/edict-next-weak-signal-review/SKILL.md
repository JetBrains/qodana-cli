---
name: edict-next-weak-signal-review
description: Review every sampled finding from one validated Edict Next candidate and materialize confident positive and negative examples.
---

# Edict Next Weak Signal Review

Load only this skill. When an example is needed, launch a fresh worker that loads `edict-next-code-example`; do not load
that skill yourself.

# Inputs and boundaries

The prompt supplies one absolute `Review config` path. Read it first, then its cluster directory, candidate inspection,
sampled findings, inspected project, and private scratch directory.

Pass the inspected IntelliJ project as `projectPath` in every Qodana MCP call, never the Edict worktree.

Do not directly edit the candidate, cluster metadata, cluster Signals, or inspected project. You may repair or delete
files below the cluster's `synthetic-examples/` directory. Code-example workers may change only their transient Signal
and the cluster's examples directory.

## 1. Establish the rule contract

Read the candidate's id, name, `htmlDescription`, and implementation together with every cluster Signal and referenced
example. Treat `htmlDescription` as the semantic rule contract and the implementation as the detector that emits findings.
Use Signals and examples only as supporting evidence for the contract and its boundaries.

## 2. Classify every finding

Read every finding. Retrieve its exact revision and range with `mcp__qodana__file_at_ref`, using `radius: 20`. Confirm
that the numbered response contains every requested range line. If it does not, retry with `radius: 5` and then
`radius: 0`; do not classify the finding until the target lines are visible. Inspect enough surrounding code and resolved
PSI to classify it:

- `TP`: the reported code violates the rule stated by `htmlDescription`.
- `FP`: the reported code does not violate that rule and must not be reported.
- `UNCERTAIN`: required evidence is genuinely unavailable or ambiguous.

Record unresolved findings and continue; every finding must receive one classification.

## 3. Materialize confident classifications

For each TP or FP, create one transient Signal in private scratch with the cluster Signal JSON shape:

- use the finding's exact `fileRevision` and a unique id;
- explain the semantic reason;
- use `Generated` source, `WEAK` strength, and `POSITIVE` for TP or `NEGATIVE` for FP;
- start with `syntheticExampleId: null`.

Launch a fresh native `spawn_agent` worker without inherited context for each transient Signal:

```text
Load the edict-next-code-example skill.

Signal path: <transient Signal path>
Synthetic examples directory: <cluster directory>/synthetic-examples
Inspected IntelliJ project: <inspected project path from review config>
```

Associate the validated example id with the finding and keep the transient Signal in private scratch. Do not create an
example for UNCERTAIN.

For each FP, also write `<private-scratch>/weak-signal-review/false-positive-<index>.md` with its path, revision, range,
exact relevant snippet, classification reason, and example id.

After every code-example worker finishes, call `mcp__qodana__edict_next_validate_cluster_examples(clusterId)`. Repair
every reported example issue and repeat validation until it succeeds. Delete incomplete unreferenced example directories;
when repairing referenced evidence, preserve its Signal's exact semantics rather than adapting it to the candidate.

## 4. Report

Write the configured output path with the sampled-findings path, reviewed and total counts, and one indexed entry per
finding. Every TP and FP includes its example id; every UNCERTAIN includes its reason and no example id; every FP includes
its report path. Return the output path.
