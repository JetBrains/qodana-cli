---
name: edict-next-weak-signal-review
description: Managed subagent that classifies sampled project findings and writes weak examples only for new evidence.
---

# Edict Next Weak Signal Review

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use its assigned task lifecycle.

Load only this skill. Do not launch workers: write every example yourself.

# Inputs and boundaries

The prompt supplies one absolute `Review config` path. Read it first, then the sampled findings, the cluster's Signals
and examples, and the inspected project as needed. Its private scratch directory is yours to write.

Take the rule contract from `inspectionDescription` in the findings file. Do not read the candidate's implementation:
classify against what the rule says, not against what the detector does.

Do not edit the candidate, cluster metadata, Signals or the inspected project. Managed state is read-only to filesystem
tools: persist examples only with `edict_next_save_code_example(token, clusterId, exampleId, metadataJson, sourceCode)`
using your own task token, and remove only your own incomplete examples with `edict_next_delete_code_example`. Derive
`clusterId` from the cluster directory name. Never assign an example to a Signal.

## 1. Evidence inventory

List every existing example: its id, label, whether a STRONG Signal references it (strong) or not (weak), and the
construct its range targets. Read `metadata.json` and the target lines; open the whole example only when needed.

## 2. Classify every finding

For each finding retrieve the exact file at its revision with read-only Git (`git show <revision>:<path>` from the
inspected project's Git root; if the object is unavailable, read it with `edict_file_at_ref` anchored at the range, with
radius 20, then 5 and 0, until every requested range line is visible) and read enough surrounding code and resolved PSI:

- `TP`: the reported code violates the rule as `inspectionDescription` states it.
- `FP`: it does not and must not be reported.
- `UNCERTAIN`: the needed evidence is genuinely unavailable or ambiguous.

Strong evidence always wins. If a finding has the same shape as a strong example but the contract would give it the
opposite label, the description is wrong, not the strong example: classify it by the strong example, record a
`contract mismatch` with both ids, and create no example for it.

## 3. Keep only new evidence

Two cases have the same shape when the reported construct is the same kind of PSI element, the parts the rule examines
have the same syntactic and resolved form, and the reason for the label is the same. Names, literals, formatting and
unrelated surrounding code do not make a shape new.

Group classified findings by shape and compare each group with the inventory:

- `FP`: one NEGATIVE example per distinct reason it is not a violation.
- `TP`: one POSITIVE example only when no existing example of either strength covers the shape.
- `UNCERTAIN`, and every case an existing example already covers: no example; record `covered-by: <exampleId>`.

Many findings of one shape are one piece of evidence. Expect to create few examples, often none.

## 4. Write the examples

Reduce the finding's exact source to one self-contained file, following the reduction rules of
[the code-example skill](../edict-next-code-example/SKILL.md) (its Signal and assignment steps do not apply):

- keep the diagnostic target and every declaration, type, call and control-flow relation that decides its label;
  replace external dependencies with minimal same-file stubs only when they preserve those semantics;
- a POSITIVE contains exactly one reportable problem and one expected range over the same semantic target;
- a NEGATIVE contains one focused allowed case and no expected ranges, and must not be trivial by removing the construct;
- never copy the complete production file; derive ranges from the finished reduced source.

Use a fresh descriptive kebab-case id. Metadata is `id`, `fileName`, `label` and `expectedRanges`. Call
`edict_next_validate_code_example(clusterId, exampleId)` and repair structural issues; after three failed repairs delete
the example and report it as not created. Finish with a successful `edict_next_validate_cluster_examples(clusterId)`.

## 5. Report

Write the configured output path with:

- the sampled-findings path, and reviewed and total counts;
- one entry per finding: index, path and line, classification, one-line reason, and either the created example id or
  `covered-by: <exampleId>` (UNCERTAIN gives its reason instead);
- every contract mismatch with its strong example id;
- the list of created example ids with label and the shape each covers.

Return the output path.
