---
name: managed-edict-next-inspection-code-review
description: Independently review a managed inspection candidate for implementation, observability, novelty, and diagnostics without state-write permissions.
---

# Edict Next Inspection Code Review

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID:`edict-next-inspection-code-review`.
Start the assigned task. This leaf has no state-write operations. Read persisted artifacts through `edict_read` /
`edict_list`; direct file writes are allowed only to the supplied private scratch review output outside the state root.
Verify the supplied candidate hash before reviewing and include it in the scratch review result. Finish the task after
returning the review outcome.

## Task contract

Accept the state-relative paths and private scratch output supplied in the prompt:

- cluster ID and state-relative directory;
- state-relative candidate inspection and its hash;
- inspected IntelliJ project;
- review output path.

Read the cluster Signals, candidate inspection, and inspected project. Review only the implementation based on available
source evidence.
Restate the candidate's general rule from the stored Signals and examples before reviewing its implementation. Treat the
motivating evidence as authoritative about the intended problem, but verify that the generalization is actually true. A
plausible detector for a misread or over-generalized rule is not acceptable.

Do not edit the candidate, cluster, examples, inspected project, or repository. Do not create examples. Base every
conclusion on available artifacts or source evidence; never turn missing information into an approval or rejection
reason.

## Review criteria

1. **Observable predicate.** Confirm that local PSI, resolution, or bounded analysis can observe every fact the rule
   depends on.
   A local inspection cannot infer revision history, runtime state, architectural intent, or out-of-file mutation unless
   the candidate
   has a sound observable proxy. Require the implementation to prove every contextual qualifier in the rule.
2. **Traversal and lookup cost.** Keep the inspection visitor bounded to the file being inspected. Ordinary symbol
   resolution, reading resolved declarations (including library APIs), and inexpensive indexed lookups outside that
   file are allowed. This includes checking superclasses/interfaces and finding inheritors; crossing a file boundary
   is not by itself a defect or a reason to reject the candidate.

   Judge searches by their scope, frequency, and result consumption. For example, `ClassInheritorsSearch` or a targeted
   `ReferencesSearch` can be appropriate after a selective syntax/symbol check, using the narrowest relevant use/module/
   project scope and stopping once the needed evidence is found. Prefer direct inheritance checks when they answer
   the question. A lazy query is not automatically cheap: avoid repeated deep hierarchy searches, eager collection of
   every project usage, whole-project PSI walks, or nested scans for each visited element. Reuse results within the
   analysis where safe; do not retain stale PSI in global caches.

   Report a performance defect with concrete evidence of the costly query, its trigger frequency, and its scope or
   observed runtime. Do not assume standard resolution or an inheritor query is expensive merely from its API name.
3. **Implementation quality and cost.** Check that helpers express auditable rule boundaries, syntax filters precede
   resolution or
   searches, and complexity is proportional to the semantic problem. Reject hard-coded seed details
   (e.g. exact files or line numbers) that do not represent a stable API or project contract.
4. **Diagnostic contract.** Ensure the rule ID is stable lowercase kebab-case, severity matches certainty, the finding
   highlights the fixable semantic unit, and the message and description state the implemented violation and an actually
   applicable remedy.

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

Use `REJECT` when any evidence-backed `BLOCKER` or `MAJOR` finding exists. Otherwise use `ACCEPT`. Do not reject or
invent a third outcome for missing evidence; state relevant limitations in the summary and decide from available
evidence.
