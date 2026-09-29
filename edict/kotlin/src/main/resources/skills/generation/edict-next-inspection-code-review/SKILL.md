---
name: edict-next-inspection-code-review
description: Managed subagent that reviews one Edict Next candidate and appends validated examples for reproducible behavioral gaps.
---

# Edict Next Inspection Code Review

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use its assigned task lifecycle.

Load only this skill.

## Inputs and boundaries

The prompt supplies absolute paths for the cluster directory, candidate inspection, inspected IntelliJ project, and
review output. Read every Signal, every synthetic example, the candidate, and relevant project source. Do not read
cluster history, predecessor inspections, or prior reviews. Pass the inspected project as `projectPath` in every Qodana
MCP call.

Do not edit the candidate, existing examples, Signals, cluster metadata, inspected project, or any other repository
file. You may append new example directories below `<cluster directory>/synthetic-examples/` and write the supplied
review output. Never assign a review example to a Signal; review examples are weak evidence.

Managed state is read-only to filesystem tools. Append each review example with
`edict_next_save_code_example(token, clusterId, exampleId, metadataJson, sourceCode)` using your own task token. Never
use `apply_patch` or shell writes below the cluster directory. The review output is private scratch and may be written
normally.

## Review

Infer the behavior best supported by the positive and negative evidence, then review the candidate:

An example referenced by a `STRONG` cluster Signal is required evidence. Every other example is weak evidence. The
generation worker has already validated the complete corpus and candidate immediately before this review.

1. **Coverage and precision.** Probe incidental restrictions and boundary conditions implied by the Signals. For every
   reproducible false negative or false positive, append one focused example as described below. Do not return an FP or
   FN as a review finding or repair suggestion.
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

## Append behavioral examples

Before appending an example, compile and run the complete candidate and reproduce the mismatch. Use a fresh descriptive
lowercase kebab-case example id and write exactly:

```text
<cluster directory>/synthetic-examples/<example-id>/metadata.json
<cluster directory>/synthetic-examples/<example-id>/project/<file-name>.kt|java
```

A false negative becomes a `POSITIVE` example containing exactly one reportable occurrence and one expected range over
that target. A false positive becomes a focused `NEGATIVE` example with no expected ranges. Keep the source
self-contained and preserve the language and semantic conditions that reproduce the mismatch. Do not add speculative,
duplicate, uncompilable, multi-problem examples.

After all additions, call `edict_next_validate_cluster_examples(clusterId)` and require success. Structural
validation does not establish the label: independently confirm the Java or Kotlin semantics and the observed candidate
result. Record appended ids only in `addedExampleIds`; do not duplicate their FP/FN descriptions in `findings` or turn
them into free-form work for the generation agent.

## Output

Write the supplied review output with exactly this shape:

```json
{
  "candidateHash": "sha256 of the complete candidate bytes",
  "status": "ACCEPT|REJECT|EXAMPLES_ADDED",
  "addedExampleIds": ["example-id"],
  "findings": [
    {
      "severity": "BLOCKER|MAJOR|MINOR",
      "category": "OBSERVABILITY|IMPLEMENTATION|PERFORMANCE|DIAGNOSTIC",
      "description": "evidence-backed issue",
      "evidence": ["artifact or source location"],
      "suggestion": "smallest implementation correction, or null"
    }
  ],
  "summary": "concise decision rationale"
}
```

Use `REJECT` for any evidenced non-behavioral BLOCKER or MAJOR finding. Use `EXAMPLES_ADDED` when examples were appended
and no such finding exists. Use `ACCEPT` only when no examples were appended and no BLOCKER or MAJOR finding exists.
Missing information is not evidence for approval or rejection: state the limitation and decide from available evidence.
