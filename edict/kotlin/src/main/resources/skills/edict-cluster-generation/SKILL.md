---
name: edict-cluster-generation
description: Generate and independently review one Pending cluster's inspection using scoped edict-mcp writes and scratch-only inspection execution.
---

# Cluster Generation

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-cluster-generation`.
Start the assigned task. Its scope is one cluster and its named inspection paths; keep cluster ID, language, membership,
and all source evidence unchanged. Persist every description/history/inspection change through MCP. Delegate all example
construction and reviews to fresh native subagents.

The parent provides the cluster ID, source project, and private scratch directory outside the state root. Run at most
**three review iterations, including the initial candidate**, across the complete code/weak/value review cycle. Include
`iteration N/3` in review task titles and the attempt manifest. Count existing code-review tasks on resumption; do not
reset the budget after a different review stage, a changed hash, or a restarted worker. One iteration permits at most
one review of each kind. Any repair after code review consumes the next iteration; do not start a fourth iteration.
The managed server refuses a fourth task of each review kind for this cluster worker, including after a restart.
An exhausted review budget is a bounded domain outcome; finalize existing work rather than retrying the rejected add.

Only BLOCKER findings require repairs. MAJOR findings request another iteration when one remains, but do not stop
example validation, project execution, or downstream reviews in the current iteration. Do not change code merely to
clear MAJOR or MINOR findings; carry them into the next review for reassessment against the collected evidence. If the
candidate is unchanged, reuse its hash-matched measurements. At the third iteration, proceed with a blocker-free,
validated candidate and record remaining MAJOR/MINOR findings as limitations. If a BLOCKER or required validation
failure remains, preserve valid Pending state and report the remaining work so the coordinator can start another
cluster. Do not waive compilation, required examples, or provenance checks to exhaust the budget successfully.

Keep at most one direct example/review child active at a time and close/dispose it after collecting its result. Other
clusters run concurrently and need their reserved descendant slots. Do not confuse a domain status with task failure: a documented valid
Pending/Invalid outcome may complete the bounded task, but missing required inputs or failed delegated tasks fail it.

## Evidence and examples

Read description, history, every signal, and referenced examples through MCP. Retrieve exact historical source when
needed. Current source evidence governs the rule; prior history preserves continuity. Refine a misleading description
through MCP with a history entry, preserving ID and all unrelated fields. Do not rename clusters in this pipeline.

For each signal without a valid assigned example, delegate `edict-code-example` with `cluster.signal.write` scoped
to that exact signal and `example.write` scoped to this cluster's examples directory. Pass the full signal path and
exact revision evidence. Verify its assignment through MCP after the child completes. Existing examples still require
structural and semantic checks.

Only incompatible semantic requirements proven by exact signal evidence justify `Discontinued`. Record the signal IDs
and contradiction. Tool failure, missing/broken evidence, duplicates, rejected candidates, and implementation limits do
not prove incompatibility.

## Candidate and measurements

Discover inspection-server tools and inspect their schemas. Request Inspection KTS API documentation/examples and PSI
evidence where relevant. Use only source reads and generic inspection execution with scratch inputs/outputs. Do not
invoke legacy Edict session, preparation, validation, or transition tools that write state. A missing compatible
compiler/runner is a concrete capability failure: record it and preserve valid partial state as Invalid; do not claim
inspection validation occurred.

Generate one general `localInspection { ... }` implementation in a single `.kts` file. Keep the inspection visitor
bounded to the inspected file. Ordinary symbol resolution and inexpensive indexed queries may read outside it,
including resolved library declarations, superclass/interface checks, and finding inheritors. Apply selective cheap
filters first, use the narrowest relevant scope, and stop queries once the needed evidence is found. Targeted reference
searches are allowed under the same cost constraints; local-only questions should use `LocalSearchScope(file)`.
Avoid whole-project PSI walks, eager collection of all usages, repeated deep hierarchy searches per visited element,
and data-flow analysis. Do not mark a coherent rule Invalid merely because a cheap lookup crosses a file boundary.

No hard-coded example paths, names, line numbers, or seed text. Use explicit imports only where the runtime does not
provide them. Persist candidate content to `inspections/<id>.candidate.kts` through MCP, preserving its returned hash.
Materialize that exact content and examples in private scratch for tools that require files. An existing predecessor may
be reused only after these same current-evidence measurements and reviews pass; never trust its prior Generated status
alone.

Preserve the full script contract from the runner's template, including the returned collection of `InspectionKts`
descriptors and diagnostic metadata. Declaring a `localInspection` variable alone does not return a runnable inspection.
For `run_inspection_kts`, populate `inspectionKtsCode` directly from the stored candidate's `edict_read` content or its
byte-identical scratch file. Do not retype, condense, or reformat it in the tool arguments. Hash the actual string sent
to the runner and record that hash with each measurement; a hash copied from a different candidate is not evidence.

1. Delegate `edict-inspection-code-review` with `operations: []`. Supply cluster ID, candidate path/hash, source
   project, scratch output, and iteration number. On a BLOCKER/REJECT, make the smallest general repair and obtain a
   fresh independent review in the next iteration, if one remains. With only MAJOR/MINOR findings, continue this
   iteration's measurements and reviews.
2. Compile the exact reviewed candidate and run it against all assigned examples with an available scratch-only
   inspection runner. Require compilation success, at least one positive example, and at least 85% aggregate label
   accuracy. A positive example must report its expected range; a negative example must not report a problem. Keep
   actual measured per-example results in scratch. Failed compilation or required accuracy/range checks are blocking;
   repair general predicates and repeat review/measurement only within the three-iteration budget.
3. Run the exact measured candidate on the inspected source project. Save complete findings and a deterministic bounded
   sample in scratch with candidate hash and revision provenance. Build an attempt manifest containing cluster ID,
   candidate path/hash, source project, full findings path, sampled findings path, private scratch directory, and
   configured review output paths, and iteration number. All review artifacts belong to this one attempt.
4. Delegate `edict-weak-signal-review` with `example.write` limited to this cluster's examples directory; this
   permission is for delegation to its example children. Supply the manifest. Read every false-positive report and its
   severity. Repair BLOCKER findings in the next iteration if available; MAJOR findings request another iteration but
   do not block the value review or eventual publication. Do not automatically repair every false positive.
5. Delegate `edict-inspection-value-review` with `operations: []`, passing the exact manifest and weak-review
   result. On a BLOCKER/REJECT, repair the supported issue and repeat affected measurements/reviews in the next
   iteration if available. With MAJOR findings but no blockers, reiterate while budget remains; after iteration three
   proceed with the validated candidate and documented limitations. With no BLOCKER or MAJOR findings, finish early.
   Rejection or exhausted iterations alone do not justify Invalid or Discontinued.

Any candidate byte change invalidates prior reviews, accuracy, and project findings. Before acceptance reread the stored
candidate and require its hash to equal every accepted review and measured run.

## Persist the outcome

Append an evidence-backed history entry with iteration count, accuracy, review decisions and severities,
source/candidate hashes, remaining non-blocking findings, and the resulting rule or blocking reason, using hash-checked
MCP writes. Then apply the chosen transition through MCP, keeping
every partially written artifact structurally valid:

- `Generated`: write the exact accepted candidate to `inspections/<id>.inspection.kts`, read back and verify it, delete
  the candidate and any distinct authorized predecessor inspection, clear `predecessorId`, and set status Generated
  last.
- `Discontinued`: record incompatible signal IDs and contradiction, delete authorized candidate/current/predecessor
  inspections, clear `predecessorId`, and set status Discontinued last.
- `Invalid`: record the concrete tooling/capability failure or broken input; preserve valid partial artifacts and
  predecessor, then set Invalid.
- `Pending`: preserve valid partial work and predecessor; record remaining work and leave Pending.

Do not remove an inspection outside your granted scope. On a write conflict or incomplete transition, stop and report
the exact persisted state instead of claiming the terminal outcome. Return IDs, paths, status, and evidence summary,
then finish the task.
