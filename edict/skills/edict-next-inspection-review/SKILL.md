---
name: edict-next-inspection-review
description: Independently review one validated Edict Next inspection candidate for rule value, specification fidelity, novelty, observability, precision, implementation quality, evidence coverage, and diagnostic usability. Use only as a fresh read-only worker launched by an Edict Next cluster-generation task before finalization.
---

# Edict Next Inspection Review

Load only this skill.

## Task contract

Accept exactly six absolute paths from the prompt:

- `Cluster directory`: read its description, Signals, examples, and history.
- `Candidate inspection`: read-only Inspection KTS that already passed example validation and project analysis.
- `Sampled findings JSON`: project findings produced for that exact candidate.
- `Weak-signal review JSON`: classification of every sampled finding for that exact candidate.
- `Inspected project`: source repository used to verify relevant APIs, existing coverage, and finding context.
- `Review output path`: the worker's only output file.

Do not edit the candidate, cluster, examples, inspected project, or repository. Do not create examples. Base every conclusion on available artifacts or source evidence; never turn missing information into an approval or rejection reason.

## Establish the evidence boundary

Read every supplied artifact. Use `INCOMPLETE` when any input is missing, the weak-signal review is incomplete, a finding lacks source evidence, or a material question cannot be resolved from the supplied repository. State exactly what evidence is missing.

Restate the candidate's general rule from the stored Signals and examples before reviewing its implementation. Treat the motivating evidence as authoritative about the intended problem, but verify that the generalization is actually true. A plausible detector for a misread or over-generalized rule is not acceptable.

## Review criteria

Review in this order:

1. **Novelty and enforcement layer.** Search existing inspection profile if available. If current inspection duplicates existing one in IDE, explain how it complements existing behaviour.
2. **Problem value.** Identify the concrete bug, cost, or repository contract. Finding count is evidence, not a threshold: a rare real defect can be valuable, while thousands of findings can mean an accepted convention or broad noise. Do not accept a taste-based rule without a clear project policy or owner-backed rationale.
3. **Observable predicate.** Confirm that local PSI, resolution, or bounded analysis can observe every fact the rule depends on. A local inspection cannot infer revision history, runtime state, architectural intent, or out-of-file mutation unless the candidate has a sound observable proxy. Require the implementation to prove every contextual qualifier in the rule.
4. **Precision boundary.** Inspect representative project findings and near-miss examples. Look for aliases, overloads, unrelated same-name APIs, unresolved symbols, tests and generated code, nested scopes, legitimate custom logic, and valid exception cases. Prefer a narrower detector with false negatives over a noisy detector. Resolution failure must fail closed, not become a match.
5. **Implementation quality and cost.** Check that helpers express auditable rule boundaries, syntax filters precede resolution or searches, traversal is bounded and file-local, and complexity is proportional to the semantic problem. Reject hard-coded seed details (e.g. exact files or line numbers) that do not represent a stable API or project contract.
6. **Evidence coverage.** Verify that stored examples cover representative positives and important negative boundaries. A complete weak-signal review with no new examples means only that its sample exposed no new negative case; it is not proof of universal precision. If new inspection found too little findings, it could mean that it became too narrow and overfit.
7. **Diagnostic contract.** Ensure the rule ID is stable lowercase kebab-case, severity matches certainty, the finding highlights the fixable semantic unit, and the message and description state the implemented violation and an actually applicable remedy.

Do not grade by source length, require perfect recall, or treat a prior inspection as proof of durable quality. Historical reviews frequently accepted narrow or imperfect candidates provisionally and later reverted high-noise rules.

## Output contract

Write `Review output path` with exactly this shape:

```json
{
  "status": "ACCEPT|REVISE|INCOMPLETE",
  "findings": [
    {
      "severity": "BLOCKER|MAJOR|MINOR",
      "category": "SPECIFICATION|DUPLICATION|VALUE|OBSERVABILITY|PRECISION|IMPLEMENTATION|COVERAGE|PERFORMANCE|DIAGNOSTIC",
      "description": "evidence-backed issue",
      "evidence": ["artifact, source location, finding index, or existing inspection"],
      "suggestion": "smallest general correction, or null when evidence is incomplete"
    }
  ],
  "summary": "concise decision rationale"
}
```

Use `INCOMPLETE` when required evidence is missing or ambiguous; list the missing evidence and do not turn it into a
candidate defect. Use `REVISE` only for actionable, evidence-backed defects. Use `ACCEPT` only when evidence is
complete and no `BLOCKER` or `MAJOR` finding remains.
