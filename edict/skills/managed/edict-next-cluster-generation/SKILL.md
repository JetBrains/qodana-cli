---
name: managed-edict-next-cluster-generation
description: Generate and independently review one Pending cluster's inspection using scoped edict-mcp writes and scratch-only inspection execution.
---

# Managed Cluster Generation

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-next-cluster-generation`.
Start the assigned task. Its scope is one cluster and its named inspection paths; keep cluster ID, language, membership,
and all source evidence unchanged. Persist every description/history/inspection change through MCP. Delegate all example
construction and reviews to fresh native subagents.

The parent provides the cluster ID, source project, and private scratch directory outside the state root. Keep a
120-minute cluster budget; continue general repairs while time remains. On deadline leave structurally valid Pending
state and report the remaining work. Do not confuse a domain status with task failure: a documented valid
Pending/Invalid outcome may complete the bounded task, but missing required inputs or failed delegated tasks fail it.

## Evidence and examples

Read description, history, every signal, and referenced examples through MCP. Retrieve exact historical source when
needed. Current source evidence governs the rule; prior history preserves continuity. Refine a misleading description
through MCP with a history entry, preserving ID and all unrelated fields. Do not rename clusters in this pipeline.

For each signal without a valid assigned example, delegate `edict-next-code-example` with `cluster.signal.write` scoped
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

1. Delegate `edict-next-inspection-code-review` with `operations: []`. Supply cluster ID, candidate path/hash, source
   project, and scratch output. On REJECT, make the smallest general repair and obtain a fresh independent review.
2. Compile the exact reviewed candidate and run it against all assigned examples with an available scratch-only
   inspection runner. Require compilation success, at least one positive example, and at least 85% aggregate label
   accuracy. A positive example must report its expected range; a negative example must not report a problem. Keep
   actual measured per-example results in scratch. Repair general predicates and repeat review/measurement when these
   checks fail.
3. Run the exact measured candidate on the inspected source project. Save complete findings and a deterministic bounded
   sample in scratch with candidate hash and revision provenance. Build an attempt manifest containing cluster ID,
   candidate path/hash, source project, full findings path, sampled findings path, private scratch directory, and
   configured review output paths. All review artifacts belong to this one attempt.
4. Delegate `edict-next-weak-signal-review` with `example.write` limited to this cluster's examples directory; this
   permission is for delegation to its example children. Supply the manifest. Read every false-positive report. Repair
   the predicate and repeat candidate review, example validation, and project execution when confident FPs exist.
5. Delegate `edict-next-inspection-value-review` with `operations: []`, passing the exact manifest and weak-review
   result. On REJECT, fix the supported general issue and repeat the affected measurements/reviews. Rejection alone does
   not justify Invalid or Discontinued.

Any candidate byte change invalidates prior reviews, accuracy, and project findings. Before acceptance reread the stored
candidate and require its hash to equal every accepted review and measured run.

## Persist the outcome

Append an evidence-backed history entry with attempts, accuracy, review decisions, source/candidate hashes, and the
resulting rule or blocking reason, using hash-checked MCP writes. Then apply the chosen transition through MCP, keeping
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
