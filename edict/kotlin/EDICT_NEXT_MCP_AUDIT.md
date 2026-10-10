# Edict Next MCP method audit

Source audited: Ultimate
`plugins/llm/qodana/agents/src/com/intellij/ml/llm/qodana/agents/edictnext/EdictNextMcpToolset.kt`
and every bundled Edict Next skill reference to its tools.

| Ultimate method | Skill use | Classification | Managed Kotlin decision |
| --- | --- | --- | --- |
| `edict_next_prepare_pipeline` | Preparation creates/loads a worktree and initializes one run session. | Pipeline/task management | Ignore. The trusted host supplies one managed state root; `edict-prepare` now freezes selected inbox bytes with `edict_validate_inbox`. |
| `edict_next_next_signal` | Distribution advances a server-owned alphabetical queue and returns same-language embedding candidates. | Pipeline/task management/retrieval | Port. The cursor and durable embedding cache are bound to the running managed distribution task and its immutable preparation receipt. |
| `edict_next_get_distribution_context` | Distribution records that candidate context was read. | Pipeline/task management | Port. Complete current cluster membership is returned and an existing-cluster context receipt is recorded for the current Signal. |
| `edict_next_add_signal_to_cluster` | Distribution applies the only legacy distribution mutation. | Pipeline/task management/state mutation | Port. The managed capability authorizes one hash-checked server transition that creates/updates the cluster, verifies the exact Signal destination, records history, and only then deletes the inbox copy. Generic writes are rejected for distribution workers. |
| `edict_next_validate_distribution` | Root calls it after distribution and stops on failure. | State validation | Port as `edict_validate_distribution`, using the immutable receipt and server-side repository snapshot from `edict_validate_inbox`. It requires each selected Signal to move byte-for-byte into exactly one cluster, rejects unrelated state changes, and validates repository invariants. |
| `edict_next_get_generation_clusters` | Generation freezes Pending targets and supplies concurrency. | Pipeline/task management | Ignore. The managed coordinator receives explicit affected/Pending IDs and controls native worker capacity. |
| `edict_next_validate_code_example` | Every code-example worker calls it before assigning the example. | State validation plus IDE parsing | Split. `edict_validate_code_example` ports persisted metadata/layout/language/label/range checks; the skill performs the PSI/parser check with Ultimate's generic inspection MCP before assignment. |
| `edict_next_validate_cluster_examples` | Overseer, cluster generation, and weak review require it after reconciliation. | State validation plus IDE parsing | Split. `edict_validate_cluster_examples` ports complete Signal/example linkage and structural state checks; skills retain Ultimate PSI/parser checks per example. |
| `edict_next_get_inspection_action` | Cluster generation detects conflicts and executes a predecessor against examples to choose reuse or generation. | Generated-inspection execution plus session deadline/action state | Do not port. Managed generation reviews and measures the previous version with the same generic `run_inspection_kts` calls as a new candidate. |
| `edict_next_validate_inspection` | Cluster generation compiles the candidate and measures all examples. | Generated-inspection execution | Keep in Ultimate. Managed skills pass exact `edict_read` candidate bytes as `inspectionKtsCode`, the inspected source project as `projectPath`, and enforce strong-example/85% accuracy gates. |
| `edict_next_get_new_inspection_results` | Cluster generation executes the candidate over the inspected project and builds weak-review inputs. | Generated-inspection execution | Keep in Ultimate. Managed skills use the same generic `run_inspection_kts` call shape and store complete findings/manifests only in private scratch. |
| `edict_next_mark_generated` | Applies a guarded legacy session transition after action, analysis, and validation receipts. | Session/task management plus state mutation | Ignore. Managed hash-checked writes publish exact accepted bytes; state validation is separate from external execution/review evidence. |
| `edict_next_validate_generation` | Root validates frozen state, then recompiles every Generated inspection before publication. | Mixed state validation and generated-inspection execution | Split. `edict_validate_generation` ports repository/target state checks. Skills must first verify Ultimate compilation, example/project runs, reviews, and exact-candidate hashes; the managed validator explicitly does not replace them. |

The resulting boundary is intentional: managed Kotlin owns authoritative state and read-only state validation; Ultimate
owns PSI parsing and all generated-inspection compilation/execution; native agents own orchestration and task lifecycle.

## TODO: pipeline review findings (2026-10-07)

End-to-end review of the current pipeline (code reading only, nothing run). Paths are relative to
`src/main/kotlin/org/jetbrains/qodana/edict/` unless they name a skill. Items marked (verified) were re-checked in code.

### Critical: a tool becomes unavailable or one failure fails the stage

- [ ] (verified) A failed IDE MCP handshake breaks every inspection tool until restart.
  `edictnext/IntellijMcpServerService.kt:145-154`: `serverLifecycle.start()` sets `helper`, then `clientFactory.create()`
  throws; `client` stays null with the helper alive, so every later `start()` fails with "IntelliJ MCP is already
  running". `restart()` has the same hole. Stop the lifecycle when `create` fails.
- [ ] (verified) A parent can finish `completed` only when every descendant completed
  (`edictnext/EdictNextRepositoryState.kt:199-237`); `edict_task_cancel` marks the child `failed` and no child can be
  abandoned. The whole plan fails on: one failed `edict-next-code-example` leaf (overseer -> cluster -> generation ->
  plan, contradicting "Pending/Invalid is not a stage failure"); a spawn failure or lost worker (protocol says cancel);
  a `task_add` without a successful `edict_delegate`; a server restart, where interrupted children return to `pending`
  and a coordinator that does not re-delegate exactly those ids can never complete.
- [ ] After the cluster deadline `edict-next-cluster-generation/SKILL.md` forbids every MCP call, including
  `edict_task_finish`; the task stays running, the coordinator must cancel it, and generation fails. The deadline
  message asks to "Cleanup ... to valid Pending state", but every generation tool, including finalise Pending, rejects
  after the deadline.
- [ ] State loading requires the state root inside a Git work tree (`edictnext/EdictRepository.kt:349-360`,
  `loadGitIgnoredFiles`). A `statePath` outside Git makes `prepare_pipeline` and `get_generation_clusters` always fail.
  A git-ignored `.edict/` drops every state file (Generated clusters "have no inspection"). The `ls-files --ignored`
  listing covers the whole project without a pathspec and can exceed the 16 MiB / 60 s `runProcess` limits. Add
  `-- <stateRoot>` and skip the listing outside Git.
- [ ] (verified) `clusterId` is never validated on cluster creation (`edictnext/EdictNextDistributionService.kt:82`,
  `edictnext/EdictRepository.kt:240-277`). `../x` or an absolute path moves the Signal out of the state tree while
  distribution "completes"; `Foo_Bar` or `a/b` leave state that `loadState`/`requireNoIssues` reject on every later
  run, with no repair tool.
- [ ] (verified) `edict_publish_signal` deduplicates only against the inbox
  (`edictnext/EdictNextRepositoryState.kt:265-275`). Re-extracting a commit whose Signal was distributed writes a second
  copy to `inbox/` and validation fails ("occurs 2 times") forever. Re-publishing while still in the inbox fails with
  "already exists with a different model" because LLM descriptions rarely match byte for byte.

### High: wrong data or Generated unreachable

- [ ] (verified) A timed-out analysis still uses up budget (`edictnext/EdictNextGenerationService.kt:196-200`): the count
  is incremented before `withTimeout(40m)`, which also covers the wait on the global `analyses` mutex. With 6 queued
  clusters a cluster can lose analyses without running any. The timeout escapes as a JSON-RPC internal error, so the
  agent never sees `remainingProjectAnalyses`.
- [ ] Timeouts cannot interrupt IDE calls: `edictnext/InspectionKtsMcpClient.kt:168` makes a blocking
  `HttpClient.send` with a 1-hour timeout. The analysis limit and the cluster deadline overrun by up to 1 hour;
  `IntellijMcpServerService.stop()` takes the `analyses` mutex first, so shutdown hangs holding `.edict-mcp.lock` and a
  restarted `mcp start` is refused. Close the client before taking the mutex.
- [ ] Codex thread limit vs fan-out: `max_concurrent_threads_per_session = 50` (`setup/CodexSetup.kt:125`) against 6
  clusters, each with an overseer, N code-example workers, reviews and a weak review with its own workers. No skill
  calls `close_agent`. A refused spawn becomes a stage failure through the parent-completion rule. Unverified whether
  finished agents keep their slot until closed.
- [ ] The spawn templates in `edict-next-weak-signal-review/SKILL.md:51-57` (and cluster-generation) start with
  `Load the ... skill.`, which `edict_delegate` rejects (`edictnext/EdictNextRepositoryState.kt:154` requires
  `$<skill>`). Passed straight to `spawn_agent`, the child has no token and `edict_next_save_code_example` rejects it.
- [ ] The PR coordinator cannot complete when everything is already covered
  (`extraction/reviews/PrAnalysis.kt:148`, `require(batches.isNotEmpty())`), although the skill says to skip covered
  ranges and that an empty selection succeeds. Batches are per server, not per task, so a stale unvalidated batch from
  an earlier attempt blocks a retry.
- [ ] PR coverage records ranges that were not fully analysed (`edictnext/EdictNextRepositoryState.kt:337-360`,
  `extraction/reviews/ReviewModels.kt`): an `endDate` of today is accepted, so later PRs that day are skipped forever;
  truncated, recency-sorted discovery can leave a range permanently uncovered. Space discovery mixes a "last updated"
  sort with `timestamp` filtering (`extraction/reviews/SpaceReviews.kt:24-37`) and may stop early.
- [ ] `edict_next_save_code_example` silently overwrites an existing example, including an assigned strong one, when
  parallel workers pick the same id (`edictnext/EdictRepository.kt:101`). Skills do not say how to choose a unique id.
- [ ] A rename after analysis breaks "submit the last analyzed candidate unchanged": the scratch
  `analyzed-candidate.kts` keeps the old `id` and fails the metadata check; changing the id yields bytes never analyzed,
  so with 0 analyses left Generated is unreachable (`edictnext/EdictNextGenerationService.kt:121-129`).

### Medium / low

- [ ] Distribution can create a cluster named like a renamed cluster's predecessor; it never checks
  `inspections/<id>.inspection.kts`, so that cluster can never be Generated ("Unexpected existing final inspection",
  `edictnext/EdictNextValidation.kt:263-266`).
- [ ] Rename after `SKIP` is not enforced; finalise Generated then always rejects.
- [ ] `withinClusterGenerationDeadline` (`edictnext/EdictNextGenerationService.kt:380`) overwrites the frozen membership
  for any cluster id passed to it, bypassing the frozen-target guard (including `clustersWithoutStrongPositiveSignal`).
- [ ] Multi-file mutations are not atomic: move before manifest write in distribution, rename, Discontinued, history
  appended before `markGenerated`. A crash leaves state that blocks every later run. The embedding cache
  (`edictnext/EdictNextNeighbourFinder.kt:76-91`) is written non-atomically and read without a size check.
- [ ] A stale-session `restart()` closes the shared client and kills other clusters' in-flight analyses, which have
  already used an analysis slot.
- [ ] Distribution tools have no token check; a second `edict_next_prepare_pipeline` silently replaces the batch,
  receipts and validation baseline.
- [ ] An unfinished plan left by a crashed manager blocks every new request ("Resume unfinished plan"); the only way out
  is deleting `<state>/.edict-mcp-current` by hand.
- [ ] The plan has no upper bound since the review-iteration cap was removed; a large run can hit the 8 MiB /
  10,000-task limits, after which every lifecycle call fails. Each save rewrites the whole plan.
- [ ] Setup: `install` has no `--config` but `mcp start` does, so a non-default port set through `--config` cannot be
  installed; a repeated `--deny` (or one equal to root/`log/agent-work`/`log/process-log`) produces a duplicate TOML key;
  `log/agent-work` is neither created nor canonicalised at install.
- [ ] No depth headroom: the deepest chain (manager -> run -> generation -> cluster -> overseer -> code-example) is 5
  against `max_depth = 5`.
- [ ] A missing GitHub token is not checked up front (`extraction/reviews/ReviewClient.kt:50-51`); unauthenticated runs
  fail midway with HTTP 403.
- [ ] Any stray file in the state tree (`.DS_Store`) fails `edict_next_validate_distribution`.
- [ ] Generation tools are registered with `server.addTool` directly and skip MCP logging and token redaction.
- [ ] Analysis findings may include the state's own synthetic examples and scratch files when they live inside the
  inspected project, and are labelled with `HEAD` while coming from the working tree.

### Skill and doc mismatches

- [ ] `edict-retrospective-signal-analysis` requires `clusters/<id>/description.json`, which no longer exists, and
  needs a whole-project runner that is not forwarded. It and `edict-git-history-signal-analysis` mention a
  non-existent `inbox.write` capability.
- [ ] `edict-next-distribution` never names `edict_next_get_distribution_context`.
- [ ] `EDICT_RUN_FLOW.md`: `maxConcurrentClusterTasks` is 6 in code, not 20; the `edict_state_write` note is stale
  (skills use `edict_publish_signal`); a rename does not keep analysis progress in every case (see above).
