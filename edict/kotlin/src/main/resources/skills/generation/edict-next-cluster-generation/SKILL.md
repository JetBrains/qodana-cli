---
name: edict-next-cluster-generation
description: Managed subagent that processes one Pending Edict Next cluster through reconciliation, generation, review, and transition.
---

# Edict Next Cluster Generation

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use it for every child delegation and task transition.

Process the supplied Pending cluster through example reconciliation, predecessor reuse or candidate generation, review,
and a valid terminal or resumable state.

The prompt supplies `clusterId`, `clusterDirectory`, the absolute worktree path, a private scratch directory, and the
inspected project. Resolve the paths before any write or MCP call. Return without changing the repository if scratch
equals or is below the worktree.

Pass the inspected IntelliJ project as `projectPath` in every Qodana MCP call, never the Edict worktree.

# Boundaries

You may directly change only:

- the target cluster directory name and `id`, `status`, and `predecessorId` in its `cluster.json`;
- appended operational decisions in its `history.md`;
- `inspections/<clusterId>.candidate.kts`, `inspections/<clusterId>.inspection.kts`, and the predecessor inspection;
- the supplied private scratch directory.

Do not change cluster membership, language, Signal evidence other than an overseer's `syntheticExampleId` assignments,
the inspected project, or another cluster.

Keep `Pending` and preserve `predecessorId` until a terminal transition.

Do not load `edict-next-code-example-overseer`, `edict-next-code-example`, `edict-next-weak-signal-review`, or
`edict-next-inspection-code-review` yourself. Launch each required skill in a fresh native `spawn_agent` worker with no
inherited conversation context.

The 120-minute cluster deadline starts at the first `edict_next_get_inspection_action` call. If an MCP response says to
clean up to Pending and stop, stop children, leave valid partial artifacts, and make no further MCP call.

# Process

## 1. Reconcile code examples

Read every Signal and launch exactly one fresh overseer:

```text
Load the edict-next-code-example-overseer skill.

Cluster directory: <clusterDirectory>
Inspected IntelliJ project: <inspected project path>
```

Require a successful `edict_next_validate_cluster_examples` result. If reconciliation is incomplete, record the concrete
reason and leave Pending. If the overseer reports an exact semantic contradiction, record its Signal evidence and apply
the Discontinued transition.

## 2. Select reuse or generation

Call `edict_next_get_inspection_action(clusterId)` before changing the cluster id or candidate.

- `CONFLICT`: record the conflicting Signal ids and mark Invalid.
- `SKIP`: the predecessor passes every strong example. Review its advisory weak-example failures. When they are
  acceptable, keep the id, record reuse, and call `edict_next_mark_generated(clusterId)`. Otherwise record why they need
  further work and leave the cluster Pending. Do not create or review a candidate.
- `GENERATE`: derive and implement a new inspection.

## 3. Implement the broadest supported rule

Read all Signals, their exact source revisions when needed, and all reconciled positive and negative examples.
Independently derive the broadest coherent code-quality rule best supported by the evidence. Separate essential
problem-causing conditions from incidental names, APIs, literals, operators, and source shapes. Do not join unrelated
predicates merely to fit the corpus.

Before the first candidate, call `mcp__qodana__generate_inspection_kts_api` and
`mcp__qodana__generate_inspection_kts_examples` for the language. Use
`mcp__qodana__generate_psi_tree` when relevant PSI structure is uncertain.

Write one complete `InspectionKts` to `inspections/<clusterId>.candidate.kts`. It must declare exactly one
`localInspection` and provide a lowercase kebab-case `id`, nonblank `name`, and nonblank `htmlDescription` that describe
the implemented behavior. Do not hard-code example paths, names, text, or ranges.

The initial cluster id is only an anchor. If the implementation's id differs, rename the target cluster directory, its
`cluster.json` id, and the candidate path together before validation. Preserve `predecessorId` under its existing id.
Use the new id and directory in every later path and call. The action and deadline follow frozen Signal membership, so
do not call inspection action again.

Implementation constraints:

- use only the Inspection KTS API and one self-contained file;
- keep PSI traversal inside the inspected file;
- directly resolve current-file references, calls, types, annotations, hierarchy facts, and constants when needed;
  resolved declarations may live elsewhere and their metadata may be read;
- do not enumerate project/module/global usages, references, inheritors, overrides, files, or index contents;
- use `LocalSearchScope` only when rooted in the current file; do not use data-flow analysis;
- keep work proportional to the current file, filter syntax before resolution, handle unresolved results conservatively,
  and preserve cancellation.

## 4. Validate and review in generation cycles

Repeat this cycle until the candidate is accepted:

1. Call `edict_next_validate_cluster_examples(clusterId)` immediately before review. When it fails, do not launch or
   consume a review. Reconcile or repair the example corpus, repair the candidate when the changed evidence requires
   it, and restart this generation cycle.
2. Call `edict_next_validate_inspection(clusterId)`. It requires every strong example to pass and reports weak-example
   results as advisory evidence. Repair every strong failure and as many weak failures as possible without compromising
   the coherent rule, then restart this cycle. Every example added by the preceding code review is mandatory for the
   next candidate revision: do not launch another review while any id from that review's `addedExampleIds` appears in
   `reportedNegativeExampleIds` or `uncoveredPositiveExampleIds`. Continue to review only when every strong and newly
   added review example passes and any other remaining weak failures are an explicit, evidence-backed decision.
3. Launch one fresh review worker:

```text
Load the edict-next-inspection-code-review skill.

Cluster directory: <clusterDirectory>
Candidate inspection: <worktree>/inspections/<clusterId>.candidate.kts
Inspected IntelliJ project: <inspected project path>
Review output path: <privateScratchDirectory>/inspection-code-review.json
```

The reviewer appends a validated weak example for every reproducible false positive or false negative instead of
returning that behavioral mismatch as a finding. When `addedExampleIds` is non-empty or status is `EXAMPLES_ADDED`,
restart this generation cycle. The new examples are measured by the two validation gates before another review.

On `REJECT`, repair only the reported non-behavioral implementation, observability, performance, or diagnostic defects,
then restart this cycle. `ACCEPT` is valid only for the exact candidate hash, with an empty `addedExampleIds`, after both
validation gates succeeded immediately before that review. Do not treat free-form FP/FN claims as repair requirements;
only stored examples participate in behavioral repair.

## 5. Review project findings

Call `edict_next_get_new_inspection_results(clusterId, privateScratchDirectory)`. Launch a
fresh weak-review worker with only:

```text
Load the edict-next-weak-signal-review skill.

Review config: <returned weak-signal-review-config path>
```

Require every finding to be classified, every TP and FP to have a validated example, and the weak-review worker's final
`edict_next_validate_cluster_examples` call to succeed. Use the classifications and advisory weak-example results to
repair as many false positives and missed positives as possible while preserving every strong example. Decide whether
any remaining weak failures are acceptable. If they are not, repeat candidate review, validation, and project analysis;
otherwise record the decision and call `edict_next_mark_generated(clusterId)`.

## 6. Other terminal states

- `Discontinued`: use only when exact Signal evidence proves semantic incompatibility. Record the conflicting Signal ids
  and contradiction, remove candidate/current and predecessor inspections, clear `predecessorId`, and set the status.
- `Invalid`: record the concrete infrastructure/tooling failure or broken input and set the status. Keep valid partial
  artifacts and `predecessorId`.
- An unfinished or rejected attempt is not by itself Invalid. Keep repairing while time remains, otherwise leave Pending.

## Price reporting

Keep direct child task titles specific to their top substage. The trusted host reports USD after execution for the
cluster worker's own generation/coordination, each direct substage including all of its descendants, and the inclusive
cluster total. It fetches the current official OpenAI Standard list rates and prices disjoint uncached-input,
cached-input, cache-write, and output tokens per response, selecting long-context rates when applicable. Reasoning is
already part of output and must not be charged twice. Runtime session receipts are authoritative and become available
only after a worker finishes.
