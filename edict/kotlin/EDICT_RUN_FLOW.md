# Edict: how a run works

The host (for example `scripts/edict-benchmark/`) installs the 12 managed skills with `qodana edict install`, starts the
Edict state server with `qodana edict mcp start --project-dir <project> --state-dir <project>/.edict --http-port 0`, and
the IntelliJ inspection server with `qodana edict linter-mcp start`. It registers both as Codex MCP servers and runs one
`codex exec` with a plain request such as `process inbox and generate new rules. Managed state: <state>. Private
scratch: <scratch>`.

The root loads only `edict_manager`. Every other skill runs in a fresh native
subagent with no inherited conversation, and every Qodana MCP call uses the inspected IntelliJ project as `projectPath`.
The state root is written only through Edict MCP tools. In the benchmark, the Edict worktree and the state root are the
same directory; there is no branch, worktree creation, commit, or push.

## Managed protocol (every task)

- root `edict_manager`
  - read `edict_registry` for the permitted call graph
  - call `edict_plan_create(request, steps)` once per server lifetime; it returns the plan and the private manager token
    - after a server restart, calling it again with the exact persisted request and steps claims the unfinished plan
      with a fresh token; interrupted tasks return to pending, completed results are kept
  - plan steps from the request; the tested plan for a full run is
    `edict-batch-signal-analysis` -> `edict-next-distribution` -> `edict-next-generation`
    (`LivePipelineTest`). `edict-next-run` is also registered as a single top-level step, see below
  - stop on any failed or uncompleted task and cancel unstarted dependent stages
  - report the plan path, produced Signal/cluster/inspection ids, Generated `knownProblems`, and Pending/Invalid clusters
- every delegation, at any depth
  1. parent calls `edict_task_add(token, skill, title)` with a child permitted by the registry
  2. parent stores the full token-free instructions with `edict_delegate(token, taskId, prompt)`; the first line is
     `$<skill>`, followed by the absolute `SKILL.md` path
  3. parent passes only the returned short launch prompt to `spawn_agent` (`fork_turns: "none"`)
  4. child calls `edict_task_get(token)`, reads its `SKILL.md`, then `edict_task_start(token, agentId, skill)`
  5. child calls `edict_task_finish(token, completed|failed, result)` after all its descendants finished; this revokes
     its capabilities. A parent uses `edict_task_cancel` for a lost child

Call graph (`skills/managed/Registry.kt`):

```text
edict_manager
├── edict-batch-signal-analysis ── edict-signal-analysis
├── edict-pr-signal-analysis ───── edict-signal-analysis
├── edict-next-run ─┬─ edict-next-distribution
│                   └─ edict-next-generation
├── edict-next-distribution
└── edict-next-generation ── edict-next-cluster-generation
                              ├── edict-next-code-example-overseer ── edict-next-code-example
                              ├── edict-next-inspection-code-review
                              └── edict-next-weak-signal-review ───── edict-next-code-example
```

## Signal extraction (optional first stage)

- `$edict-batch-signal-analysis`: bounded Git range with a commit limit
  - enumerate commits in stable order; skip root, merge, revert, automated, and non-source commits;
    id `commit-<16 chars>`
  - one `$edict-signal-analysis` worker per commit; each reads the full message, exact source, and diff and returns
    POSITIVE (before) / NEGATIVE (after) findings without rule fields
  - require exact coverage, build complete `FromCommit` records in memory, then publish each with
    `edict_state_write(inbox/<id>.json, content, expectedHash)`; identical existing content counts as success
- `$edict-pr-signal-analysis`: GitHub/Space merged reviews, chunks of at most 8 work items per `$edict-signal-analysis`
  worker, validated with `edict_validate_pr_signals` before `edict_state_write`
  - **not wired**: its tools (`edict_prepare_pr_analysis`, `edict_list_pr_analysis_items`,
    `edict_get_pr_analysis_item`, `edict_validate_pr_signals`, `edict_pr_file_*`) are not registered by the current
    server. The PR analysis code was removed in `683c9006`
- in the benchmark, extraction is skipped: the fixture's checked-in `.edict/inbox` is processed in place

## Distribution and generation

- `$edict-next-distribution`
  - call `edict_prepare_pipeline(worktreePath)` once
    - validate the source and worktree repositories and require their distribution state to match
    - select up to 100 alphabetical JVM inbox Signals
    - build 10 nearest same-language neighbours per Signal with in-JVM GTE embeddings (`EdictNextNeighbourFinder`);
      vectors are cached in `<state>/embeddings/gte-large-<revision>/`, the model in `<user cache>/JetBrains/Qodana/edict/models/`
  - repeatedly call `edict_next_signal`; it returns the complete Signal and neighbouring clusters/inbox Signals
  - for every plausible cluster, call `edict_next_get_distribution_context(kind: "cluster", id)` and compare every
    member, including negatives; optionally read a neighbouring Signal (`kind: "signal"`) or its exact revision
  - call `edict_next_add_signal_to_cluster(signalId, clusterId)` with a compatible existing id (requires the context
    receipt) or a new provisional kebab-case id; retry the same Signal when `added` is false; never split clusters
  - stop on `STOP_DISTRIBUTION`
- `edict_next_validate_distribution`: only `$edict-next-run` calls it. It validates the repository and requires exactly
  the selected Signals to have moved from the inbox to clusters, with no other changes
- `$edict-next-generation`
  - resolve a scratch root outside the worktree and call `edict_next_get_generation_clusters`
    - freeze Pending clusters and their Signal memberships; `maxConcurrentClusterTasks` is 20
  - run one `$edict-next-cluster-generation` worker per cluster, keeping up to that many active
  - leaving a cluster Pending or Invalid is not a stage failure; the coordinator never repairs worker output
  - each `$edict-next-cluster-generation` worker:
    1. launches one `$edict-next-code-example-overseer`
       - read every Signal at its exact revision (`git show`, then `file_at_ref` with radius 20/5/0)
       - infer the common rule only to guide reductions; never persist it
       - keep faithful examples; for each missing or incorrect one, launch an `$edict-next-code-example` worker that
         reduces the Signal to one self-contained file, calls `edict_next_validate_code_example`, then sets
         `syntheticExampleId`
       - positive examples contain exactly one reportable occurrence and one range; negatives contain one allowed
         occurrence and no ranges
       - keep compatible unreferenced weak examples, delete incompatible ones, and repeat
         `edict_next_validate_cluster_examples` until it succeeds
       - an exact semantic contradiction makes the cluster Discontinued; incomplete reconciliation leaves it Pending
    2. calls `edict_next_get_inspection_action(clusterId)`, starting the 120-minute cluster deadline
       - `CONFLICT`: Signals with the same file revision have different labels; record their ids and set Invalid
       - `SKIP`: the predecessor passes every strong example; review advisory weak failures, then call
         `edict_next_mark_generated` or leave Pending
       - `GENERATE`: derive the broadest coherent rule from all Signals and reconciled examples
    3. on `GENERATE`, writes one complete `inspections/<clusterId>.candidate.kts`
       - read `generate_inspection_kts_api` / `generate_inspection_kts_examples` (and `generate_psi_tree` when needed)
         from the inspection server first
       - exactly one `localInspection`; the KTS owns the kebab-case id, name, and `htmlDescription`
       - if the KTS id differs from the cluster id, rename the directory, `cluster.json` id, and candidate together;
         keep `predecessorId`. The action and deadline follow frozen Signal membership
       - current-file PSI traversal and direct resolution only; no project enumeration, non-local `LocalSearchScope`,
         or data-flow analysis
    4. repeats a generation cycle until review acceptance
       1. `edict_next_validate_cluster_examples`; on failure, repair the corpus and restart the cycle
       2. `edict_next_validate_inspection`: compile, check metadata, and measure examples
          - accepted only with at least one strong positive and every strong example correct; weak results are
            advisory
          - every example added by the previous review must pass before another review
       3. launch a fresh `$edict-next-inspection-code-review` (output `<scratch>/inspection-code-review.json`)
          - check coverage/precision, observable predicate, scope/cost, implementation, and diagnostic agreement
          - every reproducible FP/FN becomes a new weak example (`EXAMPLES_ADDED`), not a finding
          - `REJECT` for a BLOCKER or MAJOR non-behavioral finding; `ACCEPT` only for the exact candidate hash with no
            added examples
    5. calls `edict_next_get_new_inspection_results(clusterId, privateScratchDirectory)` [up to 40m] and launches
       `$edict-next-weak-signal-review`
       - classify every sampled finding as TP/FP/UNCERTAIN against `htmlDescription`
       - materialize every TP and FP through an `$edict-next-code-example` worker from a transient WEAK Signal in
         scratch; write `false-positive-<n>.md` per FP
       - end with a successful `edict_next_validate_cluster_examples`
       - the worker repairs FPs/missed positives while keeping strong examples, and repeats review, validation, and
         analysis until the remaining weak failures are an explicit decision
    6. records operational evidence in `history.md` and reaches a state
       - accepted/reused: `edict_next_mark_generated(clusterId)`; it requires a frozen, unchanged target, non-empty
         history, project analysis of the exact candidate bytes, and a passing validation
       - Discontinued: exact Signal contradiction only; remove candidate/current/predecessor inspections and clear
         `predecessorId`
       - Invalid: concrete infrastructure/tooling failure or broken input; keep valid partial artifacts
       - unfinished or deadline exceeded: leave a structurally valid Pending cluster
    - the server allows at most 3 code-review and 3 weak-review tasks per cluster worker, counted across restarts
- `edict_next_validate_generation` [read-only]: only `$edict-next-run` calls it. It validates repository state and frozen
  change boundaries, matches renamed targets by Signal membership, and revalidates every Generated inspection

## `$edict-next-run` (optional wrapper)

When the manager plans it as the single step, it runs distribution [120m], `edict_next_validate_distribution`,
generation [660m], and `edict_next_validate_generation` [40m], stops unless the result is `PUBLISH`, and reports Invalid
clusters from `history.md`. Its "commit and push the worktree" step conflicts with `edict_manager`, which forbids commit,
push, and worktree creation.

## Repository state

`cluster.json` contains only `id`, `language`, `status`, and optional `predecessorId`. Inspection metadata exists only in
the KTS.

- `Generated`: `inspections/<id>.inspection.kts` exists; candidate and predecessor are absent; history is non-empty.
- `Pending` and `Invalid`: candidate is optional; a referenced predecessor inspection exists.
- `Discontinued`: no current, candidate, or predecessor inspection exists; history is non-empty.
- no unrelated inspection files exist.

The plan, task lifecycle, and logs live under the state root and `<log-dir>/edict/`:
`edict-tasks.log` (one line per task start/finish), `edict-mcp.log`, `edict-mcp-system.log` (redacted arguments and
responses), and `tasks/<task-id>.log`.

## Differences from the Ultimate `edict-next-run` flow

- A managed plan with per-task capability tokens replaces one root skill; `edict_manager` is the entry point.
- `$edict-next-prepare` is gone. Distribution calls `edict_prepare_pipeline` itself, and no branch or worktree is
  created.
- Signal extraction (commits; PRs once wired) is part of the same pipeline.
- Acceptance requires every strong example to pass; the 85% threshold remains only in tool descriptions.
- Review iterations are capped server-side (3 per review skill per cluster worker).

## Known inconsistencies

- With the tested plan (distribution -> generation directly), `edict_next_validate_distribution` and
  `edict_next_validate_generation` are not called by anyone.
- Skills name Edict tools `mcp__qodana__edict_next_*`, but hosts register the server as `edict-mcp`.
- Some messages still say `edict_next_next_signal` / `edict_next_prepare_pipeline`; the tools are `edict_next_signal`
  and `edict_prepare_pipeline`.
- `EdictNextTimeouts.session` (900m) is defined but not enforced; the benchmark job timeout is 300 minutes.
- `scripts/edict-benchmark/README.md` says five review iterations (the server allows three), and `README.md` says 13
  managed skills (the registry has 12).
