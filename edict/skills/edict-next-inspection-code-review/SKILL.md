---
name: edict-next-inspection-code-review
description: Independently review one Edict Next inspection candidate for implementation, observability, novelty, and diagnostic quality before verification.
---

# Edict Next Inspection Code Review

Load only this skill.

## Task contract

Accept the absolute paths supplied in the prompt:

- cluster directory;
- candidate inspection;
- inspected IntelliJ project;
- review output path.

Read the cluster Signals, candidate inspection, and inspected project. Review only the implementation based on available source evidence.
Restate the candidate's general rule from the stored Signals and examples before reviewing its implementation. Treat the motivating evidence as authoritative about the intended problem, but verify that the generalization is actually true. A plausible detector for a misread or over-generalized rule is not acceptable.

Do not edit the candidate, cluster, examples, inspected project, or repository. Do not create examples. Base every conclusion on available artifacts or source evidence; never turn missing information into an approval or rejection reason.

## Review criteria

1. **Novelty and enforcement layer.** Compare the candidate with final Edict inspections in
   `<repository>/inspections/*.inspection.kts`, deriving `<repository>` from the cluster directory and excluding the
   predecessor named in `description.json`. An exact duplicate is a `BLOCKER` `DUPLICATION` finding and requires `REJECT`.
2. **Observable predicate.** Confirm that local PSI, resolution, or bounded analysis can observe every fact the rule depends on.
   A local inspection cannot infer revision history, runtime state, architectural intent, or out-of-file mutation unless the candidate
   has a sound observable proxy. Require the implementation to prove every contextual qualifier in the rule.
3. **Traversal and search scope.** Require every traversal to be bounded and file-local. Reference searches must not use a project,
   module, global, or other cross-file scope. The only permitted reference-search form is:

   ```kotlin
   val searchScope = LocalSearchScope(file)
   val references = ReferencesSearch.search(mainElement, searchScope).findAll()
   ```

   Reject any other `ReferencesSearch` usage. Reject inspections whose correctness or performance depends on resolving symbols,
   usages, or declarations outside the inspected file, or on other heavy cross-file analysis.
4. **Implementation quality and cost.** Check that helpers express auditable rule boundaries, syntax filters precede resolution or
   searches, and complexity is proportional to the semantic problem. Reject hard-coded seed details
   (e.g. exact files or line numbers) that do not represent a stable API or project contract.
5. **Diagnostic contract.** Ensure the rule ID is stable lowercase kebab-case, severity matches certainty, the finding highlights the fixable semantic unit, and the message and description state the implemented violation and an actually applicable remedy.

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
