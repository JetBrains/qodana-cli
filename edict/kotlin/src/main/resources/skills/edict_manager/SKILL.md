---
name: edict_manager
description: Plan and coordinate root user requests for managed Edict signal analysis and inspection generation. Delegated workers with a task/token must use their assigned managed skill instead.
---

# Edict Manager

Turn the user's request into a short pipeline of registered managed skills. Execute every subtask in plan
in a fresh native subagent. You own planning and coordination; children own domain work.

This skill is for the root manager only. If you already received a delegated task/token, load the assigned worker's
`SKILL.md` from the supplied absolute path and follow that skill. Shared references under `edict_manager/references/`
are a protocol library, not an instruction to invoke this manager skill.

Read [the managed execution protocol](references/protocol.md) before starting. Require an available `edict-mcp` server.
The request carries no paths: after creating the plan, call `edict_context` with the manager token for them (see the protocol). The manager capability must never enter
a child prompt or inherited conversation. An unavailable server or native subagent runtime is a failed prerequisite, not
permission to execute stages inline.

# Workflow

1. Read `edict_registry` for managed skills list. Use the registered skill IDs in MCP calls for scheduling. Do not load
   child skills yourself. Route bounded commit lists to `edict-batch-signal-analysis`, review discussions to
   `edict-pr-signal-analysis`, Git `-S`/`-G` searches for more evidence supporting selected Pending clusters
   to one `edict-git-history-signal-enrichment` task per cluster, and old-snapshot versus HEAD inspection comparisons to
   `edict-retrospective-signal-analysis`.
   Route requests to promote weak signals or seek user feedback through source fixes to `edict-promote-weak-signal`.
   Route checks of those source-fix PRs to `edict-check-weak-signal-promotion`; inspection-publication review checks
   remain owned by `edict-check-promotion`. An unqualified request to check promotion reviews, including "all Edict
   promotion reviews", retains the inspection-publication meaning. Add the weak-signal checker only for an explicit
   weak-signal/source-fix review request, an explicit request for both promotion kinds, or the default pipeline.
2. Use managed to build a list of steps required for fulfilling user request. Use availabale mcp tools if it's necessary
   to understand edict state.
3. Call `edict_plan_create(request, steps)` without a token, with the user's concrete
   request and ordered `{skill, title}` steps. The response contains `plan` and a private manager `token`; use that
   token for every subsequent manager lifecycle call. `edict_plan_get` is public, but may receive the token for caller
   attribution.
   Only one successful creation is allowed per server lifetime, even after all
   tasks finish. Never call it again once you have the token. Keep the plan ID for the current server lifetime and never
   write the plan yourself. Call `edict_context` with that token before using configured paths or defaults; its
   `scratchDirectory` belongs only to the manager.
   Initial steps may be empty when all tasks depend on target selection; add them later with `edict_task_add`.
4. For generation requests, choose Pending clusters after any requested distribution and follow the generation
   orchestration below. Add dynamically determined cluster tasks with `edict_task_add`; do not create another plan.
   Schedule `edict-check-weak-signal-promotion` as a separate top-level step in the default pipeline or when weak-signal
   review checking is requested. It may run in parallel with other signal extraction; extraction does not depend on its success.
   The checker stores only review references/states on promotions and publishes actual merged evidence as Signals
   directly in each PR's originating cluster. Do not relay feedback through extraction prompts or the inbox. A source-fix rejection
   alone does not establish that its inspection is invalid. Report check failures and continue independent stages.
5. Follow the protocol's assignment flow: supply `edict_task_add`, or `edict_delegate` for a step of the created plan,
   the complete token-free task instructions starting with the child's exact managed skill invocation, then pass its
   returned short launch `prompt` to native `spawn_agent`.
   The worker fetches the full assignment with `edict_task_get`; do not copy task instructions into the launch message.
   Spawn without inherited conversation (`fork_turns: "none"`, or `fork_context: false` in runtimes exposing that
   parameter).
   Include relevant prior-stage results in the submitted instructions. Wait for the child as the protocol describes:
   `wait_agent` with `timeout_ms: 3600000`, again after a timeout; its final message gives the child's status and
   result.
   Never print the child token or save it in a prompt file.
   Resolve the delegation's `skillPath` against the installed skills directory (the parent of this skill's directory).
   Include that absolute `SKILL.md` path in the submitted instructions; the worker also receives that skill's text from
   `edict_task_get`.
   Explicit-only children may be absent from the runtime's skill catalog; the file path is authoritative.
6. On a failed child or uncompleted task, stop dependent work. Cancel a lost worker through `edict_task_cancel`, then
   cancel unstarted dependent stages with an explicit upstream-failure reason. Let independent workers finish and
   continue independent stages selected by the pipeline. When promotion is included, always delegate `edict-promote`
   after upstream tasks are terminal, adding its task if needed, even when enrichment or generation failed or was cancelled.
   Promotion scans eligible Generated inspections independently of this run's generation results, including inspections
   from earlier runs. Keep failed and cancelled task statuses and reasons; successful promotion does not make the
   whole pipeline successful. A requested retry can re-delegate failed stages in dependency order. Durable progress
   is represented by the Edict domain artifacts, not the execution plan.
   When weak-signal promotion is included, also delegate `edict-promote-weak-signal` after upstream tasks settle,
   independently of normal promotion's result. It handles Generated inspections with insufficient referenced strong
   evidence by attempting fixes at up to five reviewed true-positive locations per inspection and opening a source PR.
7. After all stages are terminal, read the current plan and report its ID, the produced signal/cluster/inspection IDs,
   every stage result including any extraction problem summary, Generated inspections' `knownProblems`, and any Pending
   or Invalid clusters with their reasons. Completion means
   every planned task completed; a Generated inspection additionally requires the generation skill's evidence and
   review checks, not a minimum strong- or weak-example score. Pending is only an input state, not an allowed
   completed generation outcome; report interrupted or failed tasks separately. If `edict_calculate_price` is available,
   call it once with the manager token after the final plan read.
   The tool prints the full cost breakdown directly to the CI task log, including every pipeline step, each individual
   cluster task when enrichment or generation splits across clusters, generation's descendant substages, the manager's
   own cost, and the run total. It also writes the private JSON report with detailed token categories.
   Include only the run's USD and token total in the final response and refer to the printed price report for the
   breakdown; do not recreate it as a Markdown table. Do not call it while a managed task is unfinished.

Do not commit, push, create state-repository worktrees, or perform external publication directly. Publication is owned
only by a delegated registered promotion skill when the selected pipeline includes it.

# Default repository pipeline

When the user does not give a more specific goal and asks to process the repository, run the daily routine, or create
new rules, do not ask them to design a pipeline. Use `defaultSignalCount` from `edict_context` as the extraction target;
an explicit user-provided Signal count overrides it. Let generation use `edict.generation.defaultGenerationCount`; do
not add a generation count to downstream instructions when the user did not provide one specifically for generation.
Every count is an evidence-constrained target: never invent Signals, force unrelated Signals into a cluster, weaken
generation validation, or claim rules or Signals that were not produced. Report each achieved count and any shortfall.

Create these separate top-level tasks; launch review checking alongside signal extraction within available capacity.
Weak-signal checking does not gate extraction. It publishes uncovered merged evidence to the inbox;
let both checking and extraction settle before distribution and cluster selection so workers see the resulting evidence.
Check failures do not block independent stages. If the
user also requests inspection-publication checks, delegate `edict-check-promotion` separately and let its decision
workers settle before selecting clusters, since those decisions can change cluster status. Add cluster tasks once their
IDs are known and promotion after upstream attempts finish or are cancelled, including when they fail:

1. `edict-check-weak-signal-promotion`: independently check weak-signal source-fix PRs from earlier
   runs that are still active. Persist review states under `promotions`, publish covered merged corrections directly as
   processed Signals in their originating clusters, and publish all other Signals to the inbox for normal distribution.
   Run concurrently with step 2; report provider/check failures without blocking independent stages.
2. `edict-pr-signal-analysis`: pass the Signal target and call `edict_fetch_pr_batch` without a selection. The child
   repeatedly extracts and publishes every PR on the next complete uncovered UTC date until it reaches the target.
   It always finishes a selected day, so the result may exceed the target. Successful publication persists every
   processed full date in PR-analysis coverage. Zero-Signal days do not count toward the target but still advance
   coverage. Stop with a reported shortfall when repository history is exhausted or a provider problem leaves the next
   date uncovered; an empty selection after exhaustion is a successful no-op.
3. `edict-next-distribution`: assign the resulting inbox Signals to clusters. Each task assigns a bounded share of the
   run's batch; while a completed task reports remaining Signals, delegate another distribution task, one at a time.
4. Select clusters for signal enrichment and rule generation.
   For suggestions, call `edict_next_select_generation_clusters`, omitting `generationCount` unless the user supplies one.
   It ranks by distinct source files among unprocessed strong positive Signals and returns candidates and
   `maxConcurrentClusterTasks`, not a reservation or a required target set. Multiple Signals from the same source path
   count once regardless of revision. You may choose
   other Pending clusters based on the user's request and available evidence, or use explicitly requested IDs without
   calling selection. Use `maxConcurrentClusterTasks` from selection when available;
   always respecting runtime capacity and reserving room for descendants. Deduplicate your chosen IDs.
   Keep the list of clusters chosen for this run here in the manager. Pass only the assigned `clusterId` to each worker;
   track any renamed IDs from worker results for later stages and the final report.

5. `edict-git-history-signal-enrichment`: launch one top-level task per chosen cluster, passing only its `clusterId`.
   Each enrichment agent directly analyzes all member Signals and historical candidates without child workers.
   Enrich each target with strong historical evidence derived from all its gathered Signals. Wait for all enrichment
   tasks to complete before delegating generation. An empty target list creates no enrichment tasks.
   A failed or incomplete search blocks generation; still continue to promotion after settling upstream tasks.
6. `edict-next-cluster-generation`: launch one top-level task per chosen cluster after history completes, keeping up
   to the concurrency limit active. Use every original and enriched Signal.
   Valid analyzed inspections become Generated regardless of strong- or weak-example accuracy; retain failures in
   history and evaluation. Completed generation never chooses Pending. Failed or cancelled generation tasks and
   unprocessed Pending or Invalid clusters do not block publication.
7. `edict-promote`: always attempt publication of every eligible Generated inspection through the configured promotion
   target, including when enrichment or generation failed, was cancelled, or had no targets. Pass no cluster filter
   unless the user explicitly restricted publication to particular clusters; eligibility includes inspections from
   earlier runs. Missing CI or promotion configuration is a failed publication prerequisite, not a reason to omit
   this task.
8. `edict-promote-weak-signal`: attempt source fixes and feedback PRs for Generated inspections lacking referenced
   strong positive Signals from five distinct source files for normal promotion. Pass no cluster filter unless the user restricted publication. Run after normal
   promotion settles, including when it fails. The worker selects up to five reviewed true-positive locations per
   inspection, attempts focused fixes, and opens a PR describing the inspection and asking users to accept if they agree.
   Report skipped locations, validation limitations, and missing or stale review evidence.

Carry a user-provided Signal target into extraction, and a user-provided generation count plus relevant completed-stage
results into downstream task instructions. For the default pipeline, do not add a generation count: selection must
omit `generationCount` so the configured default applies. Pass your chosen IDs to history and cluster workers. Reaching the Signal target completes only
extraction; continue with generation and promotion, then report their independently achieved results. A more specific
user request overrides this default pipeline and its counts, ranges, and stages.
