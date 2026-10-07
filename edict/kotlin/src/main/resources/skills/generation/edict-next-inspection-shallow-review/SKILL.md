---
name: edict-next-inspection-shallow-review
description: Managed subagent that runs a shallow review of one Edict Next candidate without mutating its evidence or implementation.
---

# Edict Next Inspection Shallow Review

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use its assigned task lifecycle.

Load only this skill.

## Inputs and boundaries

The prompt supplies `Cluster id` and an absolute private-scratch `Review output path`. Call `edict_context`; the
candidate is `<stateDirectory>/inspections/<clusterId>.candidate.kts`. Read only the candidate: no cluster files,
Signals, examples, history, predecessor inspections, prior reviews, or project source, and no inspection tools.

Do not edit the candidate, inspected project, or repository.

## Review

This review is shallow: it catches defects visible in the candidate's code alone, cheaply, before the expensive project
analysis and weak-signal review run. It does not decide whether the inspection is correct. Do not judge coverage or
precision, look for forms or edge cases the implementation misses, or compare the rule with the evidence; those belong
to the later review. Check only:

1. **Hard-coded evidence.** Conditions must not depend on file paths, file names, line numbers, offsets, or ranges. Names
   and literals are allowed only when they denote the problem the inspection reports, such as an API it flags.
2. **Scope and cost.** PSI traversal stays in the current file. Directly resolving its references, calls, types,
   annotations, hierarchy facts, and constants is allowed, including metadata reads from declarations in other files.
   **Reject** project/module/global enumeration of usages, references, inheritors, overrides, files, or indexes.
   `LocalSearchScope` must be rooted in the current file. Data-flow analysis is not allowed. Work stays proportional to
   the file: syntax filters run before resolution, and no element triggers another whole-file traversal.
3. **Implementation practices.** Require one self-contained `InspectionKts` with exactly one `localInspection` and no
   explicit `HighlightDisplayLevel` import. Unresolved references are handled conservatively, without exceptions or
   reports. Reject file or network I/O, reflection, threads, global mutable state, swallowed cancellation, and
   exceptions used as control flow.
4. **Metadata.** The id is lowercase kebab-case; the name, message, and `htmlDescription` are nonblank, contain no
   placeholders, and name the same problem the implementation reports. Do not compare their scope in detail: a
   description broader or narrower than the implementation is not a finding.

## Output

Write the supplied review output with exactly this shape:

```json
{
  "status": "ACCEPT|REJECT",
  "findings": [
    {
      "severity": "BLOCKER|MINOR",
      "category": "HARDCODED|PERFORMANCE|IMPLEMENTATION|METADATA",
      "description": "issue visible in the candidate code",
      "evidence": ["candidate location"],
      "suggestion": "smallest implementation correction, or null"
    }
  ],
  "summary": "concise decision rationale"
}
```

A violated check is a `BLOCKER`; report every one in the same review. Record anything else worth noting as `MINOR`.
Use `REJECT` for any BLOCKER; otherwise use `ACCEPT`.
