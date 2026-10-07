---
name: edict-next-inspection-code-review
description: Managed subagent that reviews one Edict Next candidate's implementation and adds realistic corner-case weak examples.
---

# Edict Next Inspection Code Review

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use its assigned task lifecycle.

Load only this skill. Do not launch workers: write every example yourself.

## Inputs and boundaries

The prompt supplies `Cluster id` and an absolute private-scratch `Review output path`. Call `edict_context`, resolve the
cluster and its candidate inspection below its `stateDirectory`, and use its `projectDirectory` as the inspected
IntelliJ project. Read the candidate and every Signal and example. Do not read cluster history, predecessor
inspections or earlier reviews.

The output of this review is evidence: weak examples the generation worker regenerates against and the final
evaluation scores. Strong examples are required evidence; every other example is weak.

Do not edit the candidate, cluster metadata, Signals or the inspected project. Managed state is read-only to filesystem
tools: persist examples only with `edict_next_save_code_example(token, clusterId, exampleId, metadataJson, sourceCode)`
using your own task token, and delete only your own incomplete examples with `edict_next_delete_code_example`. Never
assign an example to a Signal.

## 1. Rule contract and implementation map

Treat `htmlDescription` as the rule contract and the Signals and strong examples as its fixed points. List the
implementation's decision points: each PSI element type it visits, each filter and early return, each resolution and
helper that decides whether something is reported.

Call `edict_next_validate_inspection(clusterId)` to see which examples the candidate currently fails; failing weak
examples point at decision points that are likely wrong.

## 2. Find the important corner cases

Add only the few corner cases that really matter. Adding none is a normal outcome when the candidate already handles
the important forms.

At each decision point, describe forms just inside and just outside the contract:

- forms the contract reports but the implementation probably skips (missed positives);
- forms the contract excludes but the implementation probably reports (false positives).

Keep a form only when all of these hold:

- it is a common way real code expresses the construct the rule is about, in this project or in ordinary code of the
  language and of the framework the Signals come from (one-sentence justification);
- it differs from covered forms in what the rule decides, not only in syntax: extra parentheses, negation, compound
  assignment, an additional wrapper, nesting level or operator around a covered form is not a new corner case;
- a user would notice the mistake: a missed problem in a core form, or a false report on code that is written often;
- reading the implementation gives a concrete reason, at a named decision point, to expect it decides the form wrongly;
- no existing example already covers its shape (same PSI element kind, same form of the parts the rule examines, same
  reason for the label);
- its label follows clearly from the contract and agrees with every strong example; drop ambiguous forms;
- an inspection within the implementation constraints can decide it: PSI traversal of the current file and direct
  resolution of its references, calls, types, annotations, hierarchy facts and constants. Drop forms whose correct
  handling needs data-flow or alias tracking across statements, analysis of other method bodies, project/module/global
  enumeration, or a `LocalSearchScope` not rooted in the current file.

Rank the kept forms by how commonly real code writes them, then by how likely the defect is, and keep at most five.
Record every other form you considered in `droppedForms` with the reason it was not added.

## 3. Write the examples

Write each kept form as one self-contained file following the reduction rules of
[the code-example skill](../edict-next-code-example/SKILL.md): a POSITIVE has exactly one reportable problem and one
expected range; a NEGATIVE has one focused allowed case and no expected ranges. Metadata is `id`, `fileName`, `label`
and `expectedRanges`; use a fresh descriptive kebab-case id.

For each example call `edict_next_validate_code_example(clusterId, exampleId)` and repair structural issues; use
`generate_psi_tree` to confirm the target has the PSI shape the form assumes, and `run_inspection_kts` with the example
source only to confirm it parses and executes. Ignore what the candidate reports there; the evaluation scores it later.
After three failed repairs delete the example and report it as not created. Finish with a successful
`edict_next_validate_cluster_examples(clusterId)`.

## Output

Write the supplied review output with exactly this shape:

```json
{
  "candidateHash": "sha256 of the complete candidate bytes",
  "cornerCases": [
    {
      "exampleId": "id",
      "label": "POSITIVE|NEGATIVE",
      "shape": "the form",
      "whyCommon": "where real code writes this form",
      "suspectedDefect": "decision point and why it likely decides this form wrongly"
    }
  ],
  "droppedForms": ["form and why it was not added"],
  "summary": "concise rationale"
}
```
