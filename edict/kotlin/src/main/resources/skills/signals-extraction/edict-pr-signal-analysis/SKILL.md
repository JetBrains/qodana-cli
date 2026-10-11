---
name: edict-pr-signal-analysis
description: Managed coordinator subagent that schedules chunked pull-request analysis and publishes its stored Signals.
---

# PR Review Signal Orchestrator

Run only as a delegated managed subagent. Follow [the manager protocol](../edict_manager/references/protocol.md) and
[the signal contract](../edict_manager/references/signals.md). Registry ID: `edict-pr-signal-analysis`.
You orchestrate batches, retries, publication, and coverage. Never inspect discussions, read source evidence, construct
Signals, or collect Signal payloads yourself; `edict-signal-analysis` workers own all analysis and storage.

Call `edict_context` with your own token and require its configured `reviewRepository`. A daily assignment uses its explicit `signalTarget`,
or `defaultSignalCount` when the assignment omits one. Other requests require explicit PR numbers or inclusive complete
UTC date bounds. Provider credentials and all run paths belong to the server and must not be copied into child prompts.

1. Start the managed task and call `edict_get_pr_analysis_coverage`. For a daily run, call `edict_fetch_pr_batch`
   without a selection. The server selects the next complete uncovered UTC date, prepares all review work items, and
   partitions them into chunks of at most eight. For an explicit request, pass only the requested `prNumbers`, or both
   `startDate` and `endDate`. The response is intentionally small: retain only its `batchId` and
   `availableChunkCount`. Do not list work items or inspect provider details.
2. For every zero-based chunk index below `availableChunkCount`, add an `edict-signal-analysis` child and retain its
   managed task ID for cleanup. Its stored
   assignment must contain only the absolute assigned `SKILL.md` path, the `batchId`, the `chunkIndex`, and the
   requirement to analyze and store that chunk. Do not send work-item IDs, repository identity, source paths, findings,
   or prior worker output. Delegate and launch independent chunks in parallel waves within available native capacity.
3. Wait for every launched child. A child succeeds only after `edict_store_pr_signals` accepts its complete chunk and
   its task finishes `completed`. Retry each failed, cancelled, lost, or otherwise unfinished chunk exactly once with a
   fresh delegation of the same managed task and the same batch ID and chunk index. Cancel a lost worker before
   re-delegating it. Do not inspect or repair worker output yourself. After the one retry, retain successful chunks and
   record every still-failed chunk index and keep a cumulative count across batches; one failed chunk must not discard
   successful siblings.
4. Call `edict_publish_validated_pr_signals` once after all first attempts and retries terminate, including when
   `availableChunkCount` is zero. The server publishes valid Signals from stored chunks, reports missing chunks and all
   worker/provider/storage problems, and updates date or explicit-PR coverage only when the complete batch is eligible.
   Resend no Signal models. If the receipt says coverage was updated, call `edict_get_pr_analysis_coverage` and verify
   the receipt's date ranges or PR numbers are present. Once publication returns its durable receipt, call
   `edict_task_cleanup_scratch` for every chunk task created for this batch, including completed, failed, and cancelled
   tasks; a retry reuses the same managed task ID and directory. Never clean a chunk before publication, and never clean
   tasks from another batch or coordinator. If cleanup fails, retry that cleanup once, preserve the publication receipt,
   and report the extraction task as failed rather than leaving the failure hidden.
5. For a daily run, add the publication receipt's Signal count to the cumulative result. A zero-Signal complete date
   advances coverage but does not satisfy the target. Repeat from step 1 until the target is reached, always finishing
   the selected date even when it takes the total past the target. A receipt with no analyzed date ranges and
   `selectedPrCount: 0` means review history is exhausted and is a successful no-op. Stop with a reported shortfall
   after publishing a batch with missing chunks or other problems that prevent coverage; fetching again would select
   the same uncovered date. An explicit selection processes exactly one batch.
6. Keep the task result compact and token-free. Output one line per processed UTC date as
   `YYYY-MM-DD: extracted=<published count>, failed=<failed Signal or chunk count>`. Report achieved and requested
   totals. Add one `Problems:` line only when the publication receipt reports provider, analysis, storage, or missing
   chunk problems; include counts and affected chunk indices, never Signal payloads or full IDs. Report explicit PR
   coverage analogously. After publishing all successful chunks, finish `completed` when at most half of all chunks
   handled by this coordinator remain failed after retry, including when exactly half failed. Failed child tasks remain
   visible in the plan and must be reported, but they do not by themselves force coordinator failure. Finish `failed`
   with the partial receipt only when strictly more than half of all handled chunks remain failed after retry.
