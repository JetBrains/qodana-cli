---
name: edict-next-cluster-generation
description: Managed subagent that processes one Pending Edict Next cluster through reconciliation, generation, review, and transition.
---

# Edict Next Cluster Generation

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) before
domain work and use it for every child delegation and task transition.

Process the supplied Pending cluster through example reconciliation, predecessor reuse or generation rounds, and a
valid terminal state. `Pending` is only the input state, never a generation outcome.

The prompt supplies only `clusterId`. Resolve the cluster below the `stateDirectory` of your run context. Do not copy
the state path into delegated prompts.

# Boundaries

Managed state is read-only to filesystem tools; change the target cluster only through these MCP mutations, always with
your own task token:

- `edict_next_append_cluster_history(token, clusterId, entry)` appends an operational decision to `history.md`;
- `edict_next_record_evaluation(token, clusterId)` scores the inspection that would be published on every strong and
  weak example and writes `evaluation.json`; finalising `Generated` requires it for exactly that inspection and the
  current examples;
- `edict_next_finalise_cluster(token, clusterId, status, reason)` records the reason and sets `Generated`,
  `Discontinued`, or `Invalid`, applying that status's file changes. It rejects a status whose contract
  the cluster does not meet and leaves the cluster unchanged.

Generation rounds implement, review, and rename the candidate; do not do that work yourself. Never use `apply_patch` or
shell writes for the state repository.

End with exactly one successful `edict_next_finalise_cluster` call. A rejected call changes nothing and the cluster
stays unchanged, so fix what the response names. Never finalise `Pending`. A candidate with valid compilation,
metadata, execution and the required analysis becomes `Generated` even when it fails every strong or weak example.
Record those failures and limitations in history and evaluation, not as a reason to withhold generation. If the rule
cannot be implemented reliably within the constraints, the rounds use a conservative implementation that abstains on
unsupported cases, even if it reports nothing. Only broken input or a concrete compilation, execution or tooling
failure that prevents a valid inspected artifact justifies `Invalid`; exact contradictory Signal evidence justifies
`Discontinued`.

Do not load `edict-next-code-example-overseer` or `edict-next-generation-round` yourself. Launch each required skill in
a fresh native `spawn_agent` worker with no inherited conversation context.

Name every child task exactly as follows, with `<n>` counting from 1:

- `Reconcile examples for <clusterId>` for the overseer;
- `Generation round <clusterId> <n>`.

# Process

## 1. Reconcile code examples

Launch exactly one fresh overseer:

```text
Load the edict-next-code-example-overseer skill.

Cluster id: <clusterId>
```

Require a successful `edict_next_validate_cluster_examples` result. Repair incomplete reconciliation; if broken input or
tooling prevents it, finalise `Invalid` with the concrete reason. If the overseer reports an exact semantic contradiction,
finalise `Discontinued` with its Signal evidence.

## 2. Select reuse or generation

Call `edict_next_get_inspection_action(clusterId)` before any generation round.

- `CONFLICT`: finalise `Invalid` with the conflicting Signal ids.
- `SKIP`: the predecessor passes every strong example. Call `edict_next_record_evaluation`, and finalise
  `Generated`, recording reuse; weak-example failures do not prevent reuse. Do not launch a generation round.
- `GENERATE`: run generation rounds.

## 3. Generation rounds

Each round runs in its own fresh worker, so no session carries the context of earlier rounds: it implements or repairs
the candidate, passes the shallow review, and runs one evidence round of project analysis, weak-signal review and code
review. The server bounds the analyses per cluster, so the rounds end. Launch round 1, then each next round only after
the previous one completes:

```text
Load the edict-next-generation-round skill.

Cluster id: <clusterId>
Round: <n>
Previous analyzed candidate: <Analyzed candidate from the previous round's result, or none>
Previous code review: <Code review from the previous round's result, or none>
```

A round finishes with the lines `Cluster id`, `Analyzed candidate`, `Code review`, and `Next`. Use its cluster id for
every later call and round, since a round may rename the cluster. On `Next: CONTINUE`, launch the next round. On
`STOP`, finish as below. On `INVALID` or `DISCONTINUED`, finalise that status with the round's reason. When a round
fails or is cancelled, launch one more round with the last completed round's lines; when that one fails too, finalise
`Invalid` with the failure.

After `STOP`, append the decision to history, call `edict_next_record_evaluation(token, clusterId)` as the last step
before finalising, then finalise `Generated`. Do not read or change the candidate: the round left the stored candidate
the last analyzed one, which `Generated` requires.

## 4. Other statuses

- `Discontinued`: use only when exact Signal evidence proves semantic incompatibility. Give the conflicting Signal ids
  and the contradiction. It removes the candidate and the predecessor inspection.
- `Invalid`: give the concrete infrastructure/tooling failure or broken input. Valid partial artifacts stay.
- `Pending` is not an allowed outcome. An inaccurate or incomplete detector must still become `Generated` once its
  compilation, metadata, execution, project analysis and evaluation are valid. Do not use `Invalid` or `Discontinued`
  to hide failed strong examples, poor weak coverage, low precision or low recall.
