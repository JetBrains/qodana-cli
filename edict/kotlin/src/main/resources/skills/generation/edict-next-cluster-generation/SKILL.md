---
name: edict-next-cluster-generation
description: Managed subagent that processes one Pending Edict Next cluster through reconciliation, generation, review, and transition.
---

# Edict Next Cluster Generation

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use it for every child delegation and task transition.

Process the supplied Pending cluster through example reconciliation, predecessor reuse or candidate generation, review,
and a valid terminal or resumable state.

The prompt supplies only `clusterId`. Call `edict_context` before any other MCP call or write. Resolve the cluster below
its `stateDirectory`, use its `projectDirectory` as the inspected project, and create a unique private scratch directory
below its `scratchDirectory`. Do not copy the state path into delegated prompts. Return without changing the repository
if the scratch directory equals or is below the state repository.

# Boundaries

Managed state is read-only to filesystem tools; change the target cluster only through these MCP mutations, always with
your own task token:

- `edict_next_save_candidate_inspection(token, clusterId, inspectionKtsCode)` stores and validates the complete
  candidate;
- `edict_next_rename_cluster(token, clusterId, newClusterId)` renames the cluster and its candidate, including the
  candidate's `id = "<clusterId>"`; use the new id in every later call;
- `edict_next_append_cluster_history(token, clusterId, entry)` appends an operational decision to `history.md`;
- `edict_next_finalise_cluster(token, clusterId, status, reason)` records the reason and sets `Generated`,
  `Discontinued`, `Invalid`, or keeps `Pending`, applying that status's file changes. It rejects a status whose contract
  the cluster does not meet and leaves the cluster unchanged.

Never use `apply_patch` or shell writes for the state repository; write files only in the supplied private scratch directory.

End with exactly one successful `edict_next_finalise_cluster` call. A rejected call changes nothing and the cluster
stays a Pending target, so you may fix what the response names if you still have the time and verification budget left.
When nothing more can be fixed, finalise `Pending` with what is left. The only exception is a cluster deadline response, 
after which you make no MCP call.

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

Cluster id: <clusterId>
```

Require a successful `edict_next_validate_cluster_examples` result. If reconciliation is incomplete, finalise `Pending`
with the concrete reason. If the overseer reports an exact semantic contradiction, finalise `Discontinued` with its
Signal evidence.

## 2. Select reuse or generation

Call `edict_next_get_inspection_action(clusterId)` before changing the cluster id or candidate.

- `CONFLICT`: finalise `Invalid` with the conflicting Signal ids.
- `SKIP`: the predecessor passes every strong example. Keep the id and finalise `Generated`, recording reuse;
  weak-example failures do not prevent reuse. Do not create or review a candidate.
- `GENERATE`: derive and implement a new inspection.

## 3. Implement the broadest supported rule

Read all Signals, their exact source revisions when needed, and all reconciled positive and negative examples.
Independently derive the broadest coherent code-quality rule best supported by the evidence. Separate essential
problem-causing conditions from incidental names, APIs, literals, operators, and source shapes. Do not join unrelated
predicates merely to fit the corpus.

Before the first candidate, call `generate_inspection_kts_api` and
`generate_inspection_kts_examples` for the language. Use
`generate_psi_tree` when relevant PSI structure is uncertain.

Draft in private scratch; `compile_inspection_kts` and `run_inspection_kts` on one project file are available for quick
checks. The candidate counts only once saved. It must declare exactly one
`localInspection` and provide a lowercase kebab-case `id`, nonblank `name`, and nonblank `htmlDescription` that describe
the implemented behavior. Do not hard-code example paths, names, text, or ranges.

Use the cluster id as the KTS id: validation requires them to match. The initial id is provisional; when old id does not
reflect the rule, rename the cluster with `edict_next_rename_cluster` and record the old id, new id, and reason via
edict_next_append_cluster_history.

Implementation constraints:

- use only the Inspection KTS API and one self-contained file;
- do not explicitly import `HighlightDisplayLevel`; the Inspection KTS host pre-imports it and a duplicate import
  fails compilation;
- keep PSI traversal inside the inspected file;
- directly resolve current-file references, calls, types, annotations, hierarchy facts, and constants when needed;
  resolved declarations may live elsewhere and their metadata may be read;
- do not enumerate project/module/global usages, references, inheritors, overrides, files, or index contents;
- use `LocalSearchScope` only when rooted in the current file; do not use data-flow analysis;
- keep work proportional to the current file, filter syntax before resolution, handle unresolved results conservatively,
  and preserve cancellation.

## 4. Repeat generation cycles

Strong examples, those referenced by a `STRONG` cluster Signal, define the required behavior. Every other example is
weak: it targets recall and does not define behavior. For each weak example, evaluate how applicable it is to the rule
you implement, and satisfy it when it still fits; satisfy as many as the rule allows. Never contradict strong evidence 
to satisfy a weak example. Weak failures do not block a cycle or the Generated transition.

A cycle is cheap, so repeat it as often as needed. Each failed step sends you straight back to repair and regeneration:

1. Submit the complete candidate with `edict_next_save_candidate_inspection`. It always stores the candidate as the
   latest attempt, then compiles it, checks its metadata against the cluster, and runs every example: every strong
   example must pass, and weak-example results are reported. On `REPAIR_INSPECTION`, repair and submit again; only a
   saved candidate that passed may go on to project analysis.
2. Launch a fresh shallow review worker:

   ```text
   Load the edict-next-inspection-code-review skill.

   Cluster id: <clusterId>
   Review output path: <privateScratchDirectory>/inspection-code-review.json
   ```

   It reads only the candidate and checks hard-coded evidence, scope and cost, implementation practices, and metadata. On
   `REJECT`, repair every BLOCKER and restart the cycle. On `ACCEPT`, continue with project analysis.

## 5. Review project findings

Project analysis and the weak-signal review after it are expensive, so each cluster gets a fixed number per run. Copy
the saved candidate byte for byte to `<privateScratchDirectory>/analyzed-candidate.kts`, then call
`edict_next_get_new_inspection_results(clusterId, privateScratchDirectory)`; its `remainingProjectAnalyses` says how many
remain after this one. The copy is the last analyzed candidate; replace it only before the next analysis. Launch a fresh
weak-review worker with only:

```text
Load the edict-next-weak-signal-review skill.

Review config: <returned weak-signal-review-config path>
```

Require every finding to be classified, every TP and FP to have a validated example, and the weak-review worker's final
`edict_next_validate_cluster_examples` call to succeed. Use the classifications and weak-example results to decide
whether another cycle is worth it: one that removes false positives or adds missed positives without failing a strong
example. If it is and analyses remain, repeat the generation cycles and this project review.

Otherwise, and always once `remainingProjectAnalyses` is 0 or the call fails because the limit was reached, stop
generating: `Generated` requires the stored candidate to be exactly the last analyzed one. If a later cycle saved another
candidate, save `<privateScratchDirectory>/analyzed-candidate.kts` back unchanged with
`edict_next_save_candidate_inspection`. Finalise `Generated` with the decision and the remaining weak failures.

## 6. Other statuses

- `Discontinued`: use only when exact Signal evidence proves semantic incompatibility. Give the conflicting Signal ids
  and the contradiction. It removes the candidate and the predecessor inspection.
- `Invalid`: give the concrete infrastructure/tooling failure or broken input. Valid partial artifacts stay.
- `Pending`: an unfinished or rejected attempt is not by itself Invalid. Keep repairing while time remains, otherwise
  finalise `Pending` with what is left to do.
