---
name: edict-next-inspection-value-review
description: Independently review one validated Edict Next inspection candidate for rule value, precision, and evidence coverage.
---

# Edict Next Inspection Value Review

Load only this skill.

## Task contract

Accept exactly one absolute path from the prompt:

- `Review config`: JSON review manifest.

Read the manifest first, then read the referenced cluster directory, candidate inspection, complete findings file, weak-signal
review output, and inspected project. The manifest identifies one exact candidate-analysis attempt; do not substitute paths from
another attempt. The candidate already passed example validation and project analysis.

Do not edit the candidate, cluster, examples, inspected project, or repository. Do not create examples. Base every conclusion on available artifacts or source evidence; never turn missing information into an approval or rejection reason.

## Establish the evidence boundary

Read every supplied artifact and every false-positive report linked from the weak-signal review output. Consider its unresolved findings when judging the available evidence. Missing evidence is not itself a candidate defect; report only conclusions supported by the supplied artifacts or inspected project.

Restate the candidate's general rule from the stored Signals and examples before reviewing its implementation. Treat the motivating evidence as authoritative about the intended problem, but verify that the generalization is actually true. A plausible detector for a misread or over-generalized rule is not acceptable.

## Review criteria

1. **Problem value.** Identify the concrete bug, cost, or repository contract. Finding count is evidence, not a threshold: a rare
   real defect can be valuable, while thousands of findings can mean an accepted convention or broad noise. Do not accept a taste-based
   rule without a clear project policy or owner-backed rationale.
2. **Precision boundary.** Use the complete findings file to choose several diverse findings, covering different
   files, syntax/API shapes, and contexts. For each selected finding, verify that the reported code actually satisfies the intended rule and inspect nearby valid or near-miss code in the project.

   Focus on applicable risks such as aliases, overloads, unrelated same-name APIs, unresolved symbols, tests,
   generated code, nested scopes, custom logic, and legitimate exceptions. Do not treat the selected sample as proof
   that every finding is correct. Prefer false negatives over accepting too noisy matches. An unresolved symbol or failed
   resolution must not be treated as a match.
3. **Evidence coverage.** Verify that stored examples cover representative positives and important negative boundaries. A
   weak-signal review with no false-positive reports means only that its sample exposed no new negative case; it is not proof of
   universal precision. When evidence shows that the candidate is narrower than the cluster Signals or misses part of their
   positive boundary, report a `MAJOR` `COVERAGE` finding and `REJECT`. A low finding count alone is not evidence of overfitting.

## Output contract

Write `Review output path` with exactly this shape:

```json
{
  "status": "ACCEPT|REJECT",
  "findings": [
    {
      "severity": "BLOCKER|MAJOR|MINOR",
      "category": "SPECIFICATION|DUPLICATION|VALUE|OBSERVABILITY|PRECISION|IMPLEMENTATION|COVERAGE|PERFORMANCE|DIAGNOSTIC",
      "description": "evidence-backed issue",
      "evidence": ["artifact, source location, finding index, or existing inspection"],
      "suggestion": "smallest general correction, or null when the finding cannot be fixed in the candidate"
    }
  ],
  "summary": "concise decision rationale"
}
```

Use `REJECT` when any evidence-backed `BLOCKER` or `MAJOR` finding exists. Otherwise use `ACCEPT`. Do not reject or
invent a third outcome for missing evidence; state relevant limitations in the summary and decide from available evidence.
