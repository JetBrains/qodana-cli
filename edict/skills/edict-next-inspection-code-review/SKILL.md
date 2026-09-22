---
name: edict-next-inspection-code-review
description: Review one Edict Next candidate against its Signals and examples for coverage, correctness, cost, and metadata agreement.
---

# Edict Next Inspection Code Review

Load only this skill.

## Inputs and boundaries

The prompt supplies absolute paths for the cluster directory, candidate inspection, inspected IntelliJ project, and
review output. Read every Signal, every synthetic example, the candidate, and relevant project source. Do not read
cluster history, predecessor inspections, or prior reviews.

Do not edit the candidate, cluster, examples, inspected project, or repository.

## Review

Infer the behavior best supported by the positive and negative evidence, then review the candidate:

An example referenced by a `STRONG` cluster Signal is required evidence. Every other example is weak evidence. Report
weak-example disagreements so the generation worker can decide whether to repair them, but do not reject a candidate
solely because a weak example fails. A rejection must be supported independently by the Signals, source semantics, or an
implementation defect.

1. **Coverage and precision.** Reject incidental restrictions that omit valid forms implied by the Signals. Reject
   predicates that include strong negative evidence. Passing examples alone does not prove the implementation is broad
   enough.
2. **Observable predicate.** Every condition must be observable from local PSI, direct resolution, or bounded analysis.
   Revision history, runtime state, and architectural intent require a sound observable proxy.
3. **Scope and cost.** PSI traversal stays in the current file. Directly resolving its references, calls, types,
   annotations, hierarchy facts, and constants is allowed, including metadata reads from declarations in other files.
   **Reject** project/module/global enumeration of usages, references, inheritors, overrides, files, or indexes.
   `LocalSearchScope` must be rooted in the current file. Data-flow analysis is not allowed.
4. **Implementation.** Require one self-contained `InspectionKts` with exactly one `localInspection`. Check conservative
   unresolved handling, cancellation, syntax filters before resolution, proportional complexity, and absence of
   example-specific paths, names, text, or ranges.
5. **Diagnostic agreement.** The KTS id is lowercase kebab-case, and its name, message, highlighted element, and
   `htmlDescription` accurately describe what the implementation actually reports and an applicable remedy. Flag a
   description/implementation mismatch; do not invent a replacement specification or edit either side.

## Output

Write the supplied review output with exactly this shape:

```json
{
  "status": "ACCEPT|REJECT",
  "findings": [
    {
      "severity": "BLOCKER|MAJOR|MINOR",
      "category": "OBSERVABILITY|PRECISION|IMPLEMENTATION|COVERAGE|PERFORMANCE|DIAGNOSTIC",
      "description": "evidence-backed issue",
      "evidence": ["artifact or source location"],
      "suggestion": "smallest implementation correction, or null"
    }
  ],
  "summary": "concise decision rationale"
}
```

Use `REJECT` for any evidenced BLOCKER or MAJOR finding; otherwise use `ACCEPT`. Missing information is not evidence
for approval or rejection: state the limitation and decide from available evidence.
