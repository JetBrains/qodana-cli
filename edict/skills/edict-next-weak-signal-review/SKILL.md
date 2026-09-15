---
name: edict-next-weak-signal-review
description: Run one validated Edict Next candidate over the project, review every sampled finding, and produce validated positive and negative examples for confident classifications. Use only as a fresh worker launched by an Edict Next cluster-generation task.
---

# Edict Next Weak Signal Review

Load only this skill. When an example is needed, do not load `edict-next-code-example`; mention it only in a fresh
worker prompt.

# Goal

Turn one sampled project analysis into additional validation evidence for the cluster. Review every finding against the
intended general rule. Preserve each confident classification as a validated synthetic example so later candidate repairs
must continue to satisfy all established positive and negative evidence.

# Inputs and boundaries

For every Qodana MCP call, pass the inspected IntelliJ project as `projectPath`. Never pass the Edict worktree as
`projectPath`; the worktree is repository data already loaded in the run context.

The prompt supplies exactly one absolute path:

- `Review config`: JSON manifest produced by `edict_next_get_new_inspection_results`.

Read the manifest first, then read the referenced cluster directory, candidate inspection, sampled findings file, inspected
project, and private scratch directory. The manifest identifies one exact candidate-analysis attempt.

Do not directly edit the candidate, cluster, examples, or inspected project. Code-example workers may change only examples
below the cluster's `synthetic-examples/` directory and `syntheticExampleId` in their supplied transient Signal.

## 1. Establish the intended rule

Before classifying anything, read the cluster description, history, every Signal, every referenced synthetic example, and
the complete candidate. Derive the intended rule and its positive and negative boundaries from the Signals and examples;
the cluster description is only a summary and the candidate is what is being tested. Use `mcp__qodana__file_at_ref` when
a Signal's linked code example is not enough and exact source is needed to understand its context.

## 2. Classify every project finding

Read every finding from the manifest's sampled findings file. For every finding, retrieve its exact revision and relevant range
with `mcp__qodana__file_at_ref`. Read enough surrounding code and resolve PSI or symbols when needed to decide whether the
intended inspection rule applies.

Classify each finding as:

- `TP`: the reported code genuinely violates the intended general rule.
- `FP`: the code must not be reported by that rule.
- `UNCERTAIN`: required source, semantic, or rule evidence is unavailable or genuinely ambiguous.

Do not infer a classification from path, API name, wording, the cluster description, or the candidate's behaviour alone.
Inspect every returned finding. If one cannot be classified, record it as unresolved with the reason and continue.

## 3. Materialize every confident classification

For each `TP` or `FP`, write a transient Signal below the manifest's private scratch directory using the same JSON shape as
a cluster Signal:

- use a unique id and the finding's `fileRevision`;
- describe the semantic reason for the classification precisely;
- identify the review as its generated source and set strength to `WEAK`;
- use label `POSITIVE` for `TP` and `NEGATIVE` for `FP`;
- set `syntheticExampleId` to `null` initially.

Start a fresh worker for that Signal with:

```plaintext
Load the edict-next-code-example skill.

Signal path: <transient Signal path>
Synthetic examples directory: <cluster directory>/synthetic-examples
```

Read the assigned example ID from the updated transient Signal and associate it with the finding. Keep every transient Signal
in private scratch; never copy it into the repository. Do not create a Signal or example for `UNCERTAIN` evidence.

After materializing an `FP`, also write
`<private-scratch>/weak-signal-review/false-positive-<index>.md` containing:

- the finding's path, revision, and ranges;
- the exact relevant code snippet from that revision;
- why the code must not be reported by an inspection satisfying the cluster Signals;
- the assigned synthetic example ID, whether the example was created or reused.

The extra FP report supplies repair context; do not write one for a TP.

## 4. Report the review

Write the manifest's configured output path with:

- the sampled-findings path and reviewed and total counts;
- one entry for every finding, preserving its index and classification;
- the assigned synthetic example ID for every `TP` and `FP`;
- the reason and no example ID for every `UNCERTAIN` finding;
- the absolute FP-report path for each `FP`.

Return the output path.
