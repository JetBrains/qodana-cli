---
name: edict-next-weak-signal-review
description: Managed subagent that classifies sampled project findings and writes weak examples only for new evidence.
---

# Edict Next Weak Signal Review

Run only as a delegated managed subagent. Follow [the managed task protocol](../edict_manager/references/task-protocol.md) before
domain work and use its assigned task lifecycle.

Load only this skill. Do not launch workers: write every example yourself.

# Inputs and boundaries

The prompt supplies one absolute `Review config` path. Read it first, then the sampled findings, the cluster's Signals
and examples, and the inspected project as needed. Its private scratch directory is yours to write.

Take the rule contract from `inspectionDescription` in the findings file. Do not read the candidate's implementation:
classify against what the rule says, not against what the detector does.

Do not edit the candidate, cluster metadata, Signals or the inspected project. Managed state is read-only to filesystem
tools: persist examples only with `edict_next_save_code_example(token, clusterId, exampleId, metadataJson, sourceCode)`
using your own task token, and remove with `edict_next_delete_code_example` only your own incomplete examples and weak
examples you replace in step 2. Never delete or edit a strong example. Derive `clusterId` from the cluster directory name.
Never assign an example to a Signal.

## 1. Evidence inventory

Read every existing example with `edict_next_get_cluster_examples`: its id, label, whether a STRONG Signal references
it (strong) or not (weak), and its numbered source. List for each the construct its range targets and its
`exampleStatus` from the config: `reported` says whether the analyzed inspection reports it.

Every finding is reported by the analyzed inspection, so only a reported example can represent one: an FP needs a
NEGATIVE the inspection still reports, a TP a POSITIVE it reports. An example it does not report stands for a case it no
longer flags, and a finding resembling it is a different case for the detector. Examples without a status were created
after the analysis.

## 2. Repair examples that do not reproduce their findings

`unreproducedFindings` lists examples the analyzed inspection does not report although it still reports findings assigned
to them earlier (`stillReported`). The example lost what makes that real code reported, so fix the evidence:

- weak example with `noLongerReported` > 0: split it. Keep it for the findings that are fixed, and write a new example
  (step 5) from the code of one `stillReported` finding;
- weak example with `noLongerReported` = 0: replace it. Write a new example from a `stillReported` finding's code, then
  delete the old one;
- strong example: split only; never delete or edit it.

Read each `stillReported` finding at the findings file's `projectRevision` as in step 3. If they are not all one case,
write one example per case. Move every `stillReported` finding to its new example when you record (step 6).

## 3. Classify every finding

For each finding retrieve the exact file at its revision with read-only Git (`git show <revision>:<path>` from the
inspected project's Git root; if the object is unavailable, read it with `edict_file_at_ref` anchored at the range, with
radius 20, then 5 and 0, until every requested range line is visible) and read enough surrounding code and resolved PSI:

- `TP`: the reported code violates the rule as `inspectionDescription` states it.
- `FP`: it does not and must not be reported.
- `UNCERTAIN`: the needed evidence is genuinely unavailable or ambiguous.

Strong evidence always wins. If a finding has the same shape as a strong example but the contract would give it the
opposite label, the description is wrong, not the strong example: classify it by the strong example and record a
`contract mismatch` with both ids. The strong example represents it only when the analyzed inspection reports that
example; otherwise write a new example of the strong example's label from the finding.

## 4. Keep only new evidence

Two cases have the same shape when the reported construct is the same kind of PSI element, the parts the rule examines
have the same syntactic and resolved form, and the reason for the label is the same. Names, literals, formatting and
unrelated surrounding code do not make a shape new.

Group classified findings by shape and compare each group with the inventory:

- `FP`: one NEGATIVE example per distinct reason it is not a violation.
- `TP`: the analyzed inspection already reports it, so it needs no example of its own. Assign it to the most similar
  reported POSITIVE: one that violates the rule for the same reason (the same kind of violation on the same kind of
  holder), whatever its syntax, names or the API it calls. Write a POSITIVE only when no reported POSITIVE shows that
  reason.
- `UNCERTAIN`, and every case a reported example, or one you wrote in this review, already covers: no example; record
  `covered-by: <exampleId>`.

A finding never is `covered-by` an example the analyzed inspection does not report: write a new example from it instead.

Many findings of one shape are one piece of evidence. Expect to create few examples, often none.

## 5. Write the examples

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

## 6. Record and report

Record every classified finding once, after your examples are saved, with
`edict_next_record_weak_review(token, clusterId, findingsJson)`: an array of `{path, line, classification, exampleId}`
where path is the finding's `fileRevision.path`, line the start of its first `expectedRanges` entry (0 when it has
none), classification `TP`, `FP` or `UNCERTAIN`, and exampleId the example that represents the finding: the one you
created, the one it is `covered-by`, or a reported strong example of a contract mismatch. Omit `exampleId` only for
`UNCERTAIN`. Include every `stillReported` finding of step 2 with its new example; recording a location again replaces its
earlier entry. Each example you wrote is run with the analyzed inspection then. A rejected entry names an example
that does not reproduce the finding: for one you wrote, the reduction removed what makes the inspection report the
finding, so restore it from the finding's code and save the example again; otherwise write one that does. Then record
again.

Then write the configured output path with:

- the sampled-findings path, and reviewed and total counts;
- one entry per finding: index, path and line, classification, one-line reason, and either the created example id or
  `covered-by: <exampleId>` (UNCERTAIN gives its reason instead);
- every contract mismatch with its strong example id;
- every example split or replaced in step 2, with the examples written for it;
- the list of created example ids with label and the shape each covers.

Return the output path.
