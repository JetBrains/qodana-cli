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
- `edict_next_record_evaluation(token, clusterId)` scores the inspection that would be published on every strong and
  weak example and writes `evaluation.json`; finalising `Generated` requires it for exactly that inspection and the
  current examples;
- `edict_next_finalise_cluster(token, clusterId, status, reason)` records the reason and sets `Generated`,
  `Discontinued`, `Invalid`, or keeps `Pending`, applying that status's file changes. It rejects a status whose contract
  the cluster does not meet and leaves the cluster unchanged.

Never use `apply_patch` or shell writes for the state repository; write files only in the supplied private scratch directory.

End with exactly one successful `edict_next_finalise_cluster` call. A rejected call changes nothing and the cluster
stays a Pending target, so you may fix what the response names if you still have the time and verification budget left.
When nothing more can be fixed, finalise `Pending` with what is left. The only exception is a cluster deadline response, 
after which you make no MCP call.

Do not load `edict-next-code-example-overseer`, `edict-next-code-example`, `edict-next-inspection-shallow-review`,
`edict-next-weak-signal-review`, or `edict-next-inspection-code-review` yourself. Launch each required skill in a fresh
native `spawn_agent` worker with no inherited conversation context.

Name every child task exactly as follows, with `<n>` counting from 1:

- `Reconcile examples for <clusterId>` for the overseer;
- `Shallow review <clusterId> iteration <n>`;
- `Weak-signal review <clusterId> round <n>`;
- `Code review <clusterId> round <n>`.

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
- `SKIP`: the predecessor passes every strong example. Keep the id, call `edict_next_record_evaluation`, and finalise
  `Generated`, recording reuse; weak-example failures do not prevent reuse. Do not create or review a candidate.
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
- use `LocalSearchScope` only when rooted in the current file;
- do not use data-flow analysis: decide from the inspected expression and what it directly resolves to, without
  tracking values through local variables, aliases, loop variables, or helper-call parameters, and without iterating
  to a fixed point;
- treat an unresolved reference, call, or type the decision depends on as unknown and do not report: never count it
  as a match by name, and never treat it as proof that something is absent;
- keep work proportional to the current file: visit each element a bounded number of times, with no whole-body walk
  per field, per call, or per call path; follow a same-file delegation (`this(...)`, a helper call) at most one level,
  or compute one summary per method and reuse it; filter by syntax and names before resolving; preserve cancellation;
- make the message, highlighted element, and `htmlDescription` state the property the implementation verifies, not a
  stronger one it assumes.

## 4. Repeat generation cycles

Strong examples, those referenced by a `STRONG` cluster Signal, define the required behavior. Every other example is
weak: it targets recall and does not define behavior. Satisfy every weak example that still fits the rule you implement;
never contradict strong evidence to satisfy one. Fix a failing weak example by deciding its construct correctly within
the implementation constraints. Only when you are sure the construct the decision depends on cannot be analyzed within
them (for example, the copy happens inside a call whose body the inspection may not follow), make the inspection
abstain on it: treat it as unknown and do not report. Abstain with the simplest syntactic check that recognizes the
construct, such as the relevant value being passed to a call or a delegation; never follow helpers or aliases to decide
whether to abstain. Abstaining is a last resort, not a shortcut: record in history why the construct cannot be analyzed.
A weak example that neither allowed analysis nor abstaining can satisfy without failing a strong example is a known
limitation: record it in history and do not regenerate for it. Weak failures never block a cycle or the Generated
transition.

A cycle is cheap, so repeat it as often as needed. Each failed step sends you straight back to repair and regeneration:

1. Submit the complete candidate with `edict_next_save_candidate_inspection`. It always stores the candidate as the
   latest attempt, then compiles it, checks its metadata against the cluster, and runs every example: every strong
   example must pass, and weak-example results are reported. On `REPAIR_INSPECTION`, repair and submit again; only a
   saved candidate that passed may go on to project analysis.
2. Launch a fresh shallow review worker:

   ```text
   Load the edict-next-inspection-shallow-review skill.

   Cluster id: <clusterId>
   Review output path: <privateScratchDirectory>/shallow-review-<n>.json
   ```

   It reads only the candidate and checks hard-coded evidence, scope and cost, implementation practices, and metadata. On
   `REJECT`, repair every BLOCKER and restart the cycle. On `ACCEPT`, continue with an evidence round.

## 5. Evidence rounds

Project analysis and the reviews after it are expensive, so each cluster gets a fixed number of analyses per run. Copy
the saved candidate byte for byte to `<privateScratchDirectory>/analyzed-candidate.kts`, then call
`edict_next_get_new_inspection_results(clusterId, privateScratchDirectory)`; its `remainingProjectAnalyses` says how many
remain after this one. The copy is the last analyzed candidate; replace it only before the next analysis.

1. If the response has `findingsUnchanged: true`, the project findings equal the previous round's, which were already
   reviewed: skip the weak-signal review. Otherwise launch a fresh worker with only:

   ```text
   Load the edict-next-weak-signal-review skill.

   Review config: <returned weak-signal-review-config path>
   ```

   Require every finding to be classified and its final `edict_next_validate_cluster_examples` call to succeed.
2. Then launch a fresh worker:

   ```text
   Load the edict-next-inspection-code-review skill.

   Cluster id: <clusterId>
   Review output path: <privateScratchDirectory>/inspection-code-review-<n>.json
   Previous code review: <output path of the previous round's code review, or none>
   ```

   Both reviews add weak examples and report them.
3. Save the analyzed candidate again unchanged with `edict_next_save_candidate_inspection`: its validation measures it
   against the examples this round added. If one of them fails, is not a known limitation, and analyses remain, start
   another generation cycle (section 4) against it, then another evidence round.

Otherwise, and always once `remainingProjectAnalyses` is 0 or the call fails because the limit was reached, stop
generating: `Generated` requires the stored candidate to be exactly the last analyzed one. If a later cycle saved another
candidate, save `<privateScratchDirectory>/analyzed-candidate.kts` back unchanged with
`edict_next_save_candidate_inspection`. Append the decision and the remaining weak failures to history, call
`edict_next_record_evaluation(token, clusterId)` as the last step before finalising, then finalise `Generated`.

## 6. Other statuses

- `Discontinued`: use only when exact Signal evidence proves semantic incompatibility. Give the conflicting Signal ids
  and the contradiction. It removes the candidate and the predecessor inspection.
- `Invalid`: give the concrete infrastructure/tooling failure or broken input. Valid partial artifacts stay.
- `Pending`: an unfinished or rejected attempt is not by itself Invalid. Keep repairing while time remains, otherwise
  finalise `Pending` with what is left to do.
