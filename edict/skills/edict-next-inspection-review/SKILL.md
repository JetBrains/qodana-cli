---
name: edict-next-inspection-review
description: Independently review one validated Edict Next inspection candidate for rule value, specification fidelity, novelty, observability, precision, implementation quality, evidence coverage, and diagnostic usability. Use only as a fresh read-only worker launched by an Edict Next cluster-generation task before acceptance.
---

# Edict Next Inspection Review

Load only this skill.

## Task contract

Accept exactly one absolute path from the prompt:

- `Review config`: JSON manifest produced by `edict_next_get_new_inspection_results`.

Read the manifest first, then read the referenced cluster directory, candidate inspection, complete findings file, weak-signal
review output, and inspected project. The manifest identifies one exact candidate-analysis attempt; do not substitute paths from
another attempt. The candidate already passed example validation and project analysis.

Do not edit the candidate, cluster, examples, inspected project, or repository. Do not create examples. Base every conclusion on available artifacts or source evidence; never turn missing information into an approval or rejection reason.

## Establish the evidence boundary

Read every supplied artifact and every false-positive report linked from the weak-signal review output. Consider its
unresolved findings when judging the available evidence. Missing evidence is not itself a candidate defect; report only
conclusions supported by the supplied artifacts or inspected project.

Restate the candidate's general rule from the stored Signals and examples before reviewing its implementation. Treat the motivating evidence as authoritative about the intended problem, but verify that the generalization is actually true. A plausible detector for a misread or over-generalized rule is not acceptable.

## Review criteria

Review in this order:

1. **Novelty and enforcement layer.** Compare the candidate with final Edict inspections in
   `<repository>/inspections/*.inspection.kts`, deriving `<repository>` from the cluster directory and excluding the
   predecessor named in `description.json`. Also search the inspected project for an existing IntelliJ inspection that
   implements the same rule. An exact duplicate is a `BLOCKER` `DUPLICATION` finding and requires `REJECT`.
2. **Problem value.** Identify the concrete bug, cost, or repository contract. Finding count is evidence, not a threshold: a rare
   real defect can be valuable, while thousands of findings can mean an accepted convention or broad noise. Do not accept a taste-based
   rule without a clear project policy or owner-backed rationale.
3. **Observable predicate.** Confirm that local PSI, resolution, or bounded analysis can observe every fact the rule depends on.
   A local inspection cannot infer revision history, runtime state, architectural intent, or out-of-file mutation unless the candidate
   has a sound observable proxy. Require the implementation to prove every contextual qualifier in the rule.
4. **Precision boundary.** Use the complete findings file to choose several diverse findings, covering different
   files, syntax/API shapes, and contexts. For each selected finding, verify that the reported code actually satisfies
   the intended rule and inspect nearby valid or near-miss code in the project.

   Focus on applicable risks such as aliases, overloads, unrelated same-name APIs, unresolved symbols, tests,
   generated code, nested scopes, custom logic, and legitimate exceptions. Do not treat the selected sample as proof
   that every finding is correct. Prefer false negatives over accepting too noisy matches. An unresolved symbol or failed
   resolution must not be treated as a match.
5. **Implementation quality and cost.** Check that helpers express auditable rule boundaries, syntax filters precede resolution or
   searches, traversal is bounded and file-local, and complexity is proportional to the semantic problem. Reject hard-coded seed details
   (e.g. exact files or line numbers) that do not represent a stable API or project contract.
6. **Evidence coverage.** Verify that stored examples cover representative positives and important negative boundaries. A
   weak-signal review with no false-positive reports means only that its sample exposed no new negative case; it is not proof
   of universal precision. When evidence shows that the candidate is narrower than the cluster Signals or misses part of their
   positive boundary, report a `MAJOR` `COVERAGE` finding and `REJECT`. A low finding count alone is not evidence of overfitting.
7. **Diagnostic contract.** Ensure the rule ID is stable lowercase kebab-case, severity matches certainty, the finding highlights the fixable semantic unit, and the message and description state the implemented violation and an actually applicable remedy.

Do not grade by source length, require perfect recall, or treat a prior inspection as proof of durable quality. Historical reviews frequently accepted narrow or imperfect candidates provisionally and later reverted high-noise rules.

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
