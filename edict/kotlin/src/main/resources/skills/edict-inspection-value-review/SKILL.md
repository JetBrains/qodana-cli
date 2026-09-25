---
name: edict-inspection-value-review
description: Independently review a managed validated inspection candidate for value, precision, and source-evidence coverage without state-write permissions.
---

# Edict Next Inspection Value Review

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID:
`edict-inspection-value-review`. Start the assigned task. This leaf has no state-write operations. Read persisted
artifacts through `edict_read` / `edict_list`; direct file writes are allowed only to the supplied private scratch
review output outside the state root. Verify the supplied candidate hash before reviewing and include it in the scratch
review result. Finish the task after returning the review outcome.

## Task contract

Accept exactly one absolute path from the prompt:

- `Review config`: JSON review manifest.

Read the manifest first, then read the referenced cluster directory, candidate inspection, complete findings file,
weak-signal
review output, and inspected project. The manifest identifies one exact candidate-analysis attempt; do not substitute
paths from
another attempt. The candidate already passed example validation and project analysis.

Do not edit the candidate, cluster, examples, inspected project, or repository. Do not create examples. Base every
conclusion on available artifacts or source evidence; never turn missing information into an approval or rejection
reason.

## Establish the evidence boundary

Read every supplied artifact and every false-positive report linked from the weak-signal review output. Consider its
unresolved findings when judging the available evidence. Missing evidence is not itself a candidate defect; report only
conclusions supported by the supplied artifacts or inspected project.

Restate the candidate's general rule from the stored Signals and examples before reviewing its implementation. Treat the
motivating evidence as authoritative about the intended problem, but verify that the generalization is actually true. A
plausible detector for a misread or over-generalized rule is not acceptable.

## Review criteria

1. **Problem value.** Identify the concrete bug, cost, or repository contract. Finding count is evidence, not a
   threshold: a rare
   real defect can be valuable, while thousands of findings can mean an accepted convention or broad noise. Do not
   accept a taste-based
   rule without a clear project policy or owner-backed rationale.
2. **Precision boundary.** Use the complete findings file to choose several diverse findings, covering different
   files, syntax/API shapes, and contexts. For each selected finding, verify that the reported code actually satisfies
   the intended rule and inspect nearby valid or near-miss code in the project.

   Focus on applicable risks such as aliases, overloads, unrelated same-name APIs, unresolved symbols, tests,
   generated code, nested scopes, custom logic, and legitimate exceptions. Do not treat the selected sample as proof
   that every finding is correct. Prefer false negatives over accepting too noisy matches. An unresolved symbol or
   failed
   resolution must not be treated as a match.
3. **Evidence coverage.** Verify that stored examples cover representative positives and important negative boundaries.
   A
   weak-signal review with no false-positive reports means only that its sample exposed no new negative case; it is not
   proof of
   universal precision. When evidence shows that the candidate is narrower than the cluster Signals or misses part of
   their
   positive boundary, report a `MAJOR` `COVERAGE` finding. It requests another iteration while budget remains, but does
   not by itself cause `REJECT` or prevent publication. A low finding count alone is not evidence of
   overfitting.

## Output contract

Write `Review output path` with exactly this shape:

```json
{
  "candidateHash": "<exact reviewed candidate hash>",
  "status": "ACCEPT|REJECT",
  "findings": [
    {
      "severity": "BLOCKER|MAJOR|MINOR",
      "category": "SPECIFICATION|DUPLICATION|VALUE|OBSERVABILITY|PRECISION|IMPLEMENTATION|COVERAGE|PERFORMANCE|DIAGNOSTIC",
      "description": "evidence-backed issue",
      "evidence": [
        "artifact, source location, finding index, or existing inspection"
      ],
      "suggestion": "smallest general correction, or null when the finding cannot be fixed in the candidate"
    }
  ],
  "summary": "concise decision rationale"
}
```

Use `REJECT` only for an evidence-backed `BLOCKER`; otherwise use `ACCEPT`, retaining every MAJOR/MINOR finding.
BLOCKER means a demonstrated defect invalidates the core rule, recommends an unsafe change, or fails a mandatory
validation requirement. Bounded coverage/precision gaps that leave the core rule usable are MAJOR; cosmetic
improvements are MINOR. Do not promote a finding merely to force a repair.

MAJOR findings request reassessment in the next available iteration without blocking publication. The cluster worker
has three iterations total across all review stages, including its initial candidate. Report remaining MAJOR/MINOR
findings as limitations after the last iteration; do not require them all to be fixed. Missing evidence alone is not
grounds for rejection or an invented third outcome.
