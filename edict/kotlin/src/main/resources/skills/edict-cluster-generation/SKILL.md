---
name: edict-cluster-generation
description: Process one Pending cluster through evidence, previous version reuse, candidate review, and a managed state transition.
---

# Cluster Generation

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-cluster-generation`.
Start the assigned task. Its scope is one cluster and its named inspection paths; keep cluster ID, language, membership,
and all source evidence unchanged. Persist every description/history/inspection change through MCP. Delegate all example
construction and reviews to fresh native subagents.

The goal is the same as the standalone generation workflow: one general inspection covering the supplied signals,
validated examples, and reviewed project findings. `Generated` means the exact accepted inspection is persisted;
`Discontinued` means incompatible signal semantics; `Invalid` means a concrete infrastructure/tooling or broken-input
failure; `Pending` means valid partial work remains. If `inspections/<id>.inspection.kts` already exists for a Pending
cluster, it is the previous version. Preserve it until a successful publication or a proven `Discontinued` transition.
Cluster renames and direct repository/Git writes are outside this managed task's capabilities.

The parent provides the cluster ID, source project, and private scratch directory outside the state root. Run at most
**five review iterations, including the initial candidate**, across the code and weak-signal review cycle. Include
`iteration N/5` in review task titles and the attempt manifest. Count existing code-review tasks on resumption; do not
reset the budget after a different review stage, a changed hash, or a restarted worker. One iteration permits at most
one review of each kind. Any repair after code review consumes the next iteration; do not start a sixth iteration.
The managed server refuses a sixth task of each review kind for this cluster worker, including after a restart.
An exhausted review budget is a bounded domain outcome; finalize existing work rather than retrying the rejected add.

BLOCKER findings require repairs while another iteration remains. MAJOR findings request another iteration when one
remains, but do not stop example validation, project execution, or downstream reviews in the current iteration. Do not
change code merely to clear MAJOR or MINOR findings; carry them into the next review for reassessment against the
collected evidence. If the candidate is unchanged, reuse its hash-matched measurements. At the fifth iteration, review
findings no longer block publication: if the exact candidate compiles and passes the mandatory example and provenance
checks, set the cluster to `Generated` and record every unresolved BLOCKER and MAJOR finding for that exact candidate in
`description.json` under `knownProblems`. Store published limitations with schema severity MAJOR, while preserving each
review's original severity in history and scratch. A review finding on the last iteration must never leave an otherwise
validated cluster `Pending`. Preserve Pending only for a compilation failure, mandatory example/provenance failure,
missing execution capability, deadline, or interrupted transition. Do not waive those mandatory checks to exhaust the
budget successfully.

Keep at most one direct example/review child active at a time and close/dispose it after collecting its result. Other
clusters run concurrently and need their reserved descendant slots. Do not confuse a domain status with task failure: a documented valid
Pending/Invalid outcome may complete the bounded task, but missing required inputs or failed delegated tasks fail it.

## 1. Complete the evidence

Read description, history, every signal, and referenced examples through MCP. Read prior attempt artifacts from private
scratch.
Retrieve every signal's exact historical source revision and relevant ranges with read-only Git or an available
revision reader. Current signals and exact source evidence govern the rule; prior history preserves continuity but does
not override them. Refine a misleading description through MCP with a history entry recording old/new text, reason,
and evidence, preserving ID and all unrelated fields. Do not rename clusters in this pipeline.

For each signal without a valid assigned example, delegate `edict-code-example` with `cluster.signal.write` scoped
to that exact signal and `example.write` scoped to this cluster's examples directory. Pass the full signal path and
exact revision evidence. Verify its assignment through MCP after the child completes. Existing examples still require
structural and semantic checks.

Only incompatible semantic requirements proven by exact signal evidence justify `Discontinued`. Record the signal IDs
and contradiction. Tool failure, missing/broken evidence, duplicates, rejected candidates, and implementation limits do
not prove incompatibility.

After every example worker finishes, call `edict_validate_cluster_examples(clusterId)`. Repair reported managed-state
issues through new example workers and repeat until it succeeds. The validator checks persisted structure and label
links; it does not replace the workers' Ultimate PSI/parser checks or semantic source-fidelity decisions.

## 2. Decide whether to reuse the previous version

When `inspections/<id>.inspection.kts` exists, read its exact bytes through MCP before generating a new implementation.
Check whether it expresses the shared rule and can cover the current signals and examples. If plausible, copy those
exact bytes to the candidate and run all current-evidence reviews and measurements below. Prior `Generated` status is
not acceptance. Only an unchanged previous version that passes current validation and reviews can be reused; otherwise
repair it or generate a new candidate within the remaining review budget. Preserve the previous version while the
cluster remains Pending or becomes Invalid. The legacy session's action/validation tools are replaced by these managed
checks.

## 3. Generate and measure

Discover Ultimate inspection-server tools and inspect their schemas. Request Inspection KTS API documentation/examples and PSI
evidence where relevant. Use only source reads and generic inspection execution with scratch inputs/outputs. Do not
invoke legacy Edict session, preparation, validation, or transition tools that write state. A missing compatible
compiler/runner is a concrete capability failure: record it and preserve valid partial state as Invalid; do not claim
inspection validation occurred.

Before writing the first candidate, call `generate_inspection_kts_api` and `generate_inspection_kts_examples` for the
cluster language. Call `generate_psi_tree` on representative positive/negative examples when the PSI structure is
uncertain. Always use the inspected source project as `projectPath`, never managed state or scratch.

Generate one general `localInspection { ... }` implementation in a single `.kts` file. Keep the inspection visitor
bounded to the inspected file. Ordinary symbol resolution and inexpensive indexed queries may read outside it,
including resolved library declarations, superclass/interface checks, and finding inheritors. Apply selective cheap
filters first, use the narrowest relevant scope, and stop queries once the needed evidence is found. Targeted reference
searches are allowed under the same cost constraints; local-only questions should use `LocalSearchScope(file)`.
Avoid whole-project PSI walks, eager collection of all usages, repeated deep hierarchy searches per visited element,
and data-flow analysis. Do not mark a coherent rule Invalid merely because a cheap lookup crosses a file boundary.

No hard-coded example paths, names, line numbers, or seed text. Use explicit imports only where the runtime does not
provide them. Persist candidate content to `inspections/<id>.candidate.kts` through MCP, preserving its returned hash.
Materialize that exact content and examples in private scratch for tools that require files.

Preserve the full script contract from the runner's template, including the returned collection of `InspectionKts`
descriptors and diagnostic metadata. Declaring a `localInspection` variable alone does not return a runnable inspection.
For `run_inspection_kts`, populate `inspectionKtsCode` directly from the stored candidate's `edict_read` content or its
byte-identical scratch file. Do not retype, condense, or reformat it in the tool arguments. Hash the actual string sent
to the runner and record that hash with each measurement; a hash copied from a different candidate is not evidence.

1. Delegate `edict-inspection-code-review` with `operations: []`. Supply cluster ID, candidate path/hash, source
   project, scratch output, and iteration number. On a BLOCKER/REJECT, make the smallest general repair and obtain a
   fresh independent review in the next iteration, if one remains. On iteration five, retain unresolved review findings
   for `knownProblems` and continue mandatory measurements instead of leaving Pending. With only MAJOR/MINOR findings,
   continue this iteration's measurements and reviews.
2. Compile the exact reviewed candidate and run it with Ultimate's generic `run_inspection_kts` against all assigned
   examples, always passing the inspected source project as `projectPath` and exact `edict_read` candidate bytes as
   `inspectionKtsCode`. Require compilation success, at least one positive example, and at least 85% aggregate label
   accuracy. A positive example must report its expected range; a negative example must not report a problem. Keep
   actual measured per-example results in scratch. Failed compilation or required accuracy/range checks are blocking;
   repair general predicates and repeat review/measurement only within the five-iteration budget.

## 4. Review findings

1. Run the exact measured candidate with the same Ultimate `run_inspection_kts` tool on the inspected source project.
   Pass the same exact candidate bytes and inspected project path used for example measurement. Save complete findings and a deterministic bounded
   sample in scratch with candidate hash and revision provenance. Build an attempt manifest containing cluster ID,
   candidate path/hash, source project, full findings path, sampled findings path, private scratch directory, and
   configured review output paths, and iteration number. All review artifacts belong to this one attempt.
2. Delegate `edict-weak-signal-review` with `example.write` limited to this cluster's examples directory; this
   permission is for delegation to its example children. Supply the manifest. Read every false-positive report and its
   severity. Repair BLOCKER findings in the next iteration if available; MAJOR findings request another iteration but
   do not block eventual publication. On iteration five, retain all unresolved findings for `knownProblems` and publish
   after the mandatory checks pass. Do not automatically repair every false positive. With no BLOCKER or MAJOR findings,
   finish early. Rejection or exhausted iterations alone do not justify Pending, Invalid, or Discontinued.

Any candidate byte change invalidates prior reviews, accuracy, and project findings. Before acceptance reread the stored
candidate and require its hash to equal every accepted review and measured run.

## 5. Apply the transition

Append an evidence-backed history entry with iteration count, accuracy, review decisions and severities,
source/candidate hashes, remaining non-blocking findings, and the resulting rule or blocking reason, using hash-checked
MCP writes. Then apply the chosen transition through MCP, keeping
every partially written artifact structurally valid:

- `Generated`: replace the previous version at `inspections/<id>.inspection.kts` with the exact accepted candidate,
  read back and verify it, delete the candidate, and replace `description.json`'s `knownProblems` with the unresolved
  BLOCKER and MAJOR findings from the final review cycle for that exact candidate. Normalize every published entry to
  `severity: "MAJOR"`; preserve original severities in history. Each entry has `review` (`code` or `weak-signal`),
  `category`, `description`,
  `evidence`, and nullable `suggestion`; translate a weak-signal false-positive report to category `PRECISION`. Use an
  empty array when no review findings remain, so stale problems are cleared. Clear `predecessorId` and set status Generated
  last. Apply this same
  verified transition when reusing an unchanged previous version and record the reuse decision.
- `Discontinued`: record incompatible signal IDs and contradiction, delete the authorized candidate and current
  inspection (the previous version), clear `predecessorId`, and set status Discontinued last.
- `Invalid`: record the concrete tooling/capability failure or broken input; preserve valid partial artifacts and the
  previous version, then set Invalid.
- `Pending`: preserve valid partial work and the previous version only when compilation, a mandatory example/provenance
  check, a required execution capability, the deadline, or an interrupted transition prevents publication; record
  remaining work and leave Pending. Review findings alone never justify this transition after the final iteration.

Do not remove an inspection outside your granted scope. On a write conflict or incomplete transition, stop and report
the exact persisted state instead of claiming the terminal outcome. Return IDs, paths, status, and evidence summary,
then finish the task.
