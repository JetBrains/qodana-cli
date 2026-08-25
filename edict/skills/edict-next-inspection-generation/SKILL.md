---
name: edict-next-inspection-generation
description: Generate and iteratively improve a Qodana Inspection KTS script using Qodana MCP validation, project results, and delegated problem verification. Use only when explicitly invoked for an active Qodana generation session.
---

# Qodana Inspection Generation

Generate one IntelliJ IDEA local inspection in Kotlin with the Inspection KTS API. The prompt supplies the generation session ID, current specification path, final output path, inspected project path, maximum iterations, an optional initial candidate, the persistent session log path, the cumulative verification state path, and the per-iteration verification limits.

## Required Workflow

1. Read the current specification and cumulative verification state. If an initial candidate path is supplied, use that code as the first candidate; otherwise generate the first candidate from the specification.
2. Before writing a new candidate, call `mcp__qodana__generate_inspection_kts_api` and `mcp__qodana__generate_inspection_kts_examples` for the specification language. Call `mcp__qodana__generate_psi_tree` on representative positive and negative snippets whenever the relevant PSI structure is uncertain.
3. Create a complete `inspection.kts` candidate that implements the general rule. Never hardcode example file names, package names, class names, or line numbers.
4. Call `mcp__qodana__review_inspection` exactly once for the generation, apply useful general feedback, and continue with the reviewed candidate.
5. Call `mcp__qodana__validate_generated_inspection` with the session ID and the complete candidate code. If compilation or examples fail, revise the general implementation and validate again. Do not request project results until validation succeeds.
6. Call `mcp__qodana__get_inspection_results` with the session ID and the exact validated code. It returns every result from running the inspection on the current project; it does not select or verify results and does not change the specification.
7. Perform the complete iteration procedure below in the main skill. The main skill must select the problems, delegate their verification to subagents, merge and persist verification state, compute metrics, update and snapshot the specification, and choose the next action.

Every candidate written to the final output must therefore have passed both MCP stages without any source change between the project-results call and the file write.

## Project-result Iteration

The main skill owns orchestration and all file writes. Verification subagents are analysis-only: they inspect assigned findings and return classifications, but they never choose findings and never edit the verification state, specification, logs, candidate, or final output.

For each successful `get_inspection_results` response:

1. Create `<persistent-session-log-path>/iterationN`, where `N` is the returned iteration. Save the complete tool response as `project-results.json`.
2. Read the current specification and cumulative `verification-state.json`. A finding matches an existing problem when paths are equal and either side has no ranges, at least one inclusive line range intersects, or the nearest endpoints of two ranges are at most two lines apart. The small tolerance accounts for PSI anchors that move between a seeded example and the generated inspection result. Ignore project results without a path when selecting verification work.
3. The main skill chooses up to the prompt's `Problems to verify per iteration` unmatched findings. Select deterministically in tool-result order while preferring one finding per file before filling remaining slots. Do not let a subagent expand, replace, or reprioritize this list.
4. Partition the chosen findings into balanced batches and dispatch up to three verification subagents in parallel. Give every subagent the inspection description, inspected project path, and its exact assigned findings. Require it to inspect the relevant source context and return only a JSON array with `path`, `startLine`, `endLine`, `classification`, and `reason`. `classification` must be `TP` when the reported code violates the general rule and `FP` otherwise. Wait for all verifier subagents.
5. As the main skill, merge the returned classifications into cumulative state. Preserve earlier entries and add one entry per newly verified finding with this schema:

   ```json
   {
     "path": "relative/project/path",
     "revision": "HEAD",
     "expectedProblemRanges": [{"start": 10, "end": 12}],
     "classification": "TP",
     "iteration": 1,
     "reason": "short evidence-based explanation"
   }
   ```

   Use `null` for `expectedProblemRanges` when no start line is available. Write the complete `{ "version": 1, "problems": [...] }` document to the prompt's verification state path, then copy the same document to `iterationN/verification-state.json`.
6. Reproduce the previous project-feedback metrics from the persisted state:
   - Gold positives are all `TP` state entries; gold negatives are all `FP` state entries. Iteration-zero entries seeded by the host represent the original optional and synthetic examples.
   - A gold positive matching a current project result is a true positive; a gold positive with no current match is a false negative; a gold negative matching a current result is a false positive.
   - `precision = TP / (TP + FP)` and `recall = TP / (TP + FN)`, using `1.0` when a denominator is zero. `F1` is the harmonic mean, or `0.0` when precision plus recall is zero.
7. Compare those metrics with `optionalPrecisionThreshold`, `optionalRecallThreshold`, and `optionalF1Threshold` in the current specification. These fields are written explicitly by the host. Never treat a missing or null threshold as an automatic pass; for compatibility, use `0.8`, `0.7`, and `0.1` respectively if a field is absent. When any threshold is missed:
   - append at most `Examples to append per iteration` false-negative entries not already present to `inspectionSpec.optionalPositiveExamples`;
   - append at most that many false-positive entries not already present to `inspectionSpec.optionalNegativeExamples`;
   - specification examples contain only `path`, `revision`, and `expectedProblemRanges`, not verification metadata.
8. Write the resulting complete specification back to the current specification path on every iteration, even when unchanged. Copy it to `iterationN/specification.json`.
9. Write `iterationN/workflow.log` containing the selected count, TP/FP/FN counts, precision/recall/F1, thresholds, whether the specification changed, and the two snapshot paths. Append these stable event lines to `<persistent-session-log-path>/orchestration.log` as well. The host already writes lifecycle events to this file, so never truncate, replace, rewrite, or delete it:

   ```text
   verification state written: iteration=N path=... selected=... TP=... FP=... FN=...
   specification snapshot written: iteration=N path=... changed=true|false precision=... recall=... F1=...
   ```

10. Choose the next action:
    - If all metrics meet their thresholds, write the exact analyzed code to the final output path and finish.
    - If the specification did not gain any examples, keep the best exact analyzed code, write it to the final output path, and finish.
    - If the specification changed and another iteration remains, reread it, revise the candidate for the new examples, and repeat validation and project analysis.
    - If the specification changed on the last allowed iteration, do not write the final output file; generation is exhausted.
    - If an MCP tool reports `success=false`, do not write the final output file.

Use a JSON-aware editor or a short script for state and specification updates. Never update JSON with textual search-and-replace. Snapshot state and specification before moving to the next candidate so a failed later iteration remains diagnosable.

## Inspection Requirements

- Use only the Inspection KTS API and define the inspection with `localInspection { ... }`.
- Put all code in one file.
- Do not explicitly import symbols documented as Inspection KTS default imports. Import every other symbol used.
- End the file with a `listOf(InspectionKts(...))` expression that registers the local tool.
- Use `LocalSearchScope(file)` for reference searches. Global or project-wide reference search is prohibited.
- Do not generate an inspection that requires project-wide search or data-flow analysis.
- Treat failing examples as evidence about the general rule, not as values to special-case.
- Write only Kotlin source to the final output file, without Markdown fences or explanations.
