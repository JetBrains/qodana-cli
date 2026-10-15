// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.extraction.reviews

import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewExtractionApi
import org.jetbrains.qodana.edict.ci.ReviewFetchResult
import org.jetbrains.qodana.edict.ci.ReviewMessage
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalProvenance
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.numberedLines
import org.jetbrains.qodana.edict.signals.SignalValidation
import java.time.LocalDate
import java.time.ZoneOffset
import java.time.temporal.ChronoUnit

/** Prepared PR evidence and stored Signal batches scoped to one managed PR-analysis task. */
internal class PrAnalysis(
  private val store: EdictNextRepositoryState,
  private val provider: ReviewExtractionApi,
  private val today: () -> LocalDate = { LocalDate.now(ZoneOffset.UTC) },
) {
  private data class Batch(
    val summary: PrBatchSummary,
    val items: List<PrItem>,
    val chunks: List<List<PrItem>>,
    val repository: ReviewRepository,
    val selectedPrCount: Int,
    val selectedPrNumbers: List<Int>,
    val analyzedDateRanges: List<PrAnalysisDateRange>,
    val providerProblems: List<String>,
    val storedChunks: MutableMap<Int, StoredChunk> = mutableMapOf(),
    var published: Boolean = false,
  )

  private data class StoredChunk(
    val signals: Map<String, EdictNextSignal>,
    val failures: List<PrSignalStoreFailure>,
    val problems: List<String>,
    val successfulWorkItemIds: Set<String>,
  )

  private val batches = mutableMapOf<String, Batch>()

  fun prepareBatch(token: String, repository: ReviewRepository, input: ReviewSelectionInput): PrBatchSummary {
    store.requirePrAnalysisCaller(token, coordinator = true)
    input.validate()
    return when {
      input.prNumbers.isNotEmpty() -> prepareNumberBatch(token, repository, input.prNumbers)
      input.startDate.isNotEmpty() -> prepareDateBatch(token, repository, input.startDate, input.endDate)
      else -> prepareDefaultBatch(token, repository)
    }
  }

  /**
   * Selects the next complete uncovered UTC date, newest first. A successful publication covers that whole date, so
   * another default call advances to the preceding uncovered date. The current UTC date is never selected because it
   * is not complete yet.
   */
  private fun prepareDefaultBatch(token: String, repository: ReviewRepository): PrBatchSummary {
    val coverage = store.getPrAnalysisCoverage(token, repository)
    val date = generateSequence(today().minusDays(1)) { it.minusDays(1) }
      .takeWhile { !it.isBefore(MIN_REVIEW_DATE) }
      .firstOrNull { !coverage.includes(it) }
      ?: return registerBatch(token, repository, emptyList(), "${json.encodeToString(repository)}:history-exhausted")
    val fetched = fetchCompleteRange(repository, date, date)
    val prs = fetched.pullRequests
      .filterNot { it.number in coverage.analyzedPrNumbers }
      .sortedByDescending(PullRequest::closeTimestamp)
    val ranges = listOf(PrAnalysisDateRange(date.toString(), date.toString()))
    return registerBatch(
      token,
      repository,
      prs,
      json.encodeToString(repository) + json.encodeToString(ranges),
      ranges,
      fetched.problems,
    )
  }

  private fun prepareDateBatch(
    token: String,
    repository: ReviewRepository,
    startDate: String,
    endDate: String,
  ): PrBatchSummary {
    val start = LocalDate.parse(startDate)
    val end = LocalDate.parse(endDate)
    require(end.isBefore(today())) { "Only complete UTC dates before today can be selected" }
    val fetched = fetchCompleteRange(repository, start, end)
    val ranges = listOf(PrAnalysisDateRange(startDate, endDate))
    return registerBatch(
      token,
      repository,
      fetched.pullRequests.sortedByDescending(PullRequest::closeTimestamp),
      json.encodeToString(repository) + json.encodeToString(ranges),
      ranges,
      fetched.problems,
    )
  }

  private fun prepareNumberBatch(
    token: String,
    repository: ReviewRepository,
    prNumbers: List<Int>,
  ): PrBatchSummary {
    val selection = ReviewSelection(
      repository.provider,
      repository.owner,
      repository.repository,
      maxPrs = prNumbers.size,
      prNumbers = prNumbers,
    )
    selection.validate()
    val fetched = provider.fetch(selection)
    return registerBatch(
      token,
      repository,
      fetched.pullRequests,
      json.encodeToString(selection),
      problems = fetched.problems,
    )
  }

  private fun fetchCompleteRange(
    repository: ReviewRepository,
    startDate: LocalDate,
    endDate: LocalDate,
  ): ReviewFetchResult {
    val selection = ReviewSelection(
      repository.provider,
      repository.owner,
      repository.repository,
      FULL_DATE_FETCH_LIMIT,
      startDate = startDate.toString(),
      endDate = endDate.toString(),
    )
    val fetched = provider.fetch(selection)
    val prs = fetched.pullRequests
    require(prs.all { selection.containsDate(it.closeTimestamp) }) { "Provider returned a PR outside the requested dates" }
    if (prs.size < FULL_DATE_FETCH_LIMIT) return fetched
    require(startDate != endDate) {
      "Cannot prove that UTC date $startDate was fetched completely: it contains at least $FULL_DATE_FETCH_LIMIT PRs"
    }
    val midpoint = startDate.plusDays(ChronoUnit.DAYS.between(startDate, endDate) / 2)
    val newer = fetchCompleteRange(repository, midpoint.plusDays(1), endDate)
    val older = fetchCompleteRange(repository, startDate, midpoint)
    return ReviewFetchResult(
      newer.pullRequests + older.pullRequests,
      newer.problems + older.problems,
    )
  }

  private fun registerBatch(
    token: String,
    repository: ReviewRepository,
    prs: List<PullRequest>,
    selectionIdentity: String,
    analyzedDateRanges: List<PrAnalysisDateRange> = emptyList(),
    problems: List<String> = emptyList(),
  ): PrBatchSummary {
    val items = prs.flatMap { pr ->
      pr.threads.map { thread ->
        val identity = buildJsonArray {
          add(wireJson.encodeToJsonElement(repository))
          add(pr.url)
          add(pr.number)
          add(thread.threadId)
          add(thread.filePath)
          add(thread.anchorLine)
          add(thread.anchorEndLine)
        }
        val id = "pr-${pr.number}-${sha256(wireJson.encodeToString(identity)).take(16)}"
        PrItem(id, repository, pr.copy(threads = emptyList()), thread)
      }
    }
    require(items.distinctBy(PrItem::workItemId).size == items.size) { "Duplicate provider work item" }
    val id = sha256(selectionIdentity + json.encodeToString(prs) + json.encodeToString(problems)).take(24)
    val chunks = items.chunked(CHUNK_SIZE)
    val summary = PrBatchSummary(id, chunks.size)
    return synchronized(store) {
      store.requirePrAnalysisCaller(token, coordinator = true)
      batches.getOrPut(id) {
        Batch(
          summary = summary,
          items = items,
          chunks = chunks,
          repository = repository,
          selectedPrCount = prs.size,
          selectedPrNumbers = prs.map(PullRequest::number),
          analyzedDateRanges = analyzedDateRanges,
          providerProblems = problems,
        )
      }.summary
    }
  }

  fun chunk(token: String, batchId: String, chunkIndex: Int): PrChunk = synchronized(store) {
    store.requireSkill(token, setOf("edict-signal-analysis"))
    val batch = batch(token, batchId, coordinator = false)
    require(chunkIndex in batch.chunks.indices) { "Unknown chunk index in PR batch" }
    PrChunk(batchId, chunkIndex, batch.chunks[chunkIndex].map(PrItem::workItemId))
  }

  fun get(token: String, batchId: String, itemId: String): PrItem = synchronized(store) {
    store.requireSkill(token, setOf("edict-signal-analysis"))
    batch(token, batchId, coordinator = false).items.firstOrNull { it.workItemId == itemId }
      ?: error("Unknown work item in PR batch")
  }

  fun store(
    token: String,
    batchId: String,
    chunkIndex: Int,
    inspected: List<String>,
    failedWorkItemIds: List<String>,
    signals: List<EdictNextSignal>,
    problems: List<String>,
    drafts: List<PrSignalDraft> = emptyList(),
  ): PrReceipt = synchronized(store) {
    store.requireSkill(token, setOf("edict-signal-analysis"))
    store.requireTokenFree(problems.joinToString("\n"))
    val batch = batch(token, batchId, coordinator = false)
    check(!batch.published) { "Cannot store a PR chunk after batch publication" }
    require(chunkIndex in batch.chunks.indices) { "Unknown chunk index in PR batch" }
    val chunk = batch.chunks[chunkIndex]
    require(inspected == chunk.map(PrItem::workItemId)) { "Incomplete or unordered PR chunk coverage" }
    require(failedWorkItemIds.distinct().size == failedWorkItemIds.size && failedWorkItemIds.all(inspected::contains)) {
      "Failed work-item IDs must be distinct members of the inspected chunk"
    }
    require(problems.isEmpty() == failedWorkItemIds.isEmpty()) {
      "Every failed work item requires a problem, and problems require failed work-item IDs"
    }
    val stored = linkedMapOf<String, EdictNextSignal>()
    val failures = mutableListOf<PrSignalStoreFailure>()
    val completed = drafts.mapIndexedNotNull { draftIndex, draft ->
      val index = signals.size + draftIndex
      runCatching { IndexedValue(index, complete(draft, chunk, batchId)) }.onFailure { e ->
        failures += PrSignalStoreFailure(index, draft.id, draft.workItemId, e.message ?: e.javaClass.simpleName)
      }.getOrNull()
    }
    (signals.withIndex() + completed).forEach { (index, candidate) ->
      try {
        require(wireJson.encodeToString(candidate).toByteArray().size <= MAX_ARTIFACT_BYTES) { "Signal exceeds 8 MiB" }
        val signal = SignalValidation.validate(candidate)
        val item = chunk.firstOrNull { it.workItemId == signal.provenance.workItemId }
          ?: error("Signal work item does not belong to this chunk")
        require(signal.provenance.analysisBatchId == batchId) { "Signal provenance must preserve the analysis batch" }
        require(signal.id !in stored) { "Duplicate signal" }
        val source = signal.source
        require(
          source is EdictNextSignalSource.FromPR && source.prNumber == item.pr.number &&
            source.title == item.pr.title && source.url == item.thread.reviewDiscussionUrl,
        ) { "Signal source must preserve prepared PR number, title and discussion URL" }
        require(source.discussionMessages == item.thread.messages.map(ReviewMessage::body)) {
          "Preserve all prepared discussion messages in order"
        }
        require(
          if (signal.label == EdictNextSignalLabel.NEGATIVE) signal.fileRevision.revision == item.pr.headRevision
          else signal.fileRevision.revision in listOf(item.pr.baseRevision, item.thread.originalCommitSha),
        ) { "Evidence revision does not match its PR side" }
        stored[signal.id] = signal
      }
      catch (e: Exception) {
        failures += PrSignalStoreFailure(
          signalIndex = index,
          signalId = candidate.id,
          workItemId = candidate.provenance.workItemId,
          message = e.message ?: e.javaClass.simpleName,
        )
      }
    }
    val failedIds = failedWorkItemIds.toSet() + failures.map(PrSignalStoreFailure::workItemId)
    val successfulIds = inspected.toSet() - failedIds
    batch.storedChunks[chunkIndex] = StoredChunk(stored, failures, problems, successfulIds)
    PrReceipt(
      batchId = batchId,
      chunkIndex = chunkIndex,
      inspectedWorkItemCount = inspected.size,
      successfulWorkItemCount = successfulIds.size,
      failedWorkItemCount = failedIds.size,
      signalCount = stored.size,
      signalIds = stored.keys.toList(),
      failures = failures,
      problems = problems,
    )
  }

  /** Builds the draft's `FromPR` source from its prepared work item, so the worker copies none of it. */
  private fun complete(draft: PrSignalDraft, chunk: List<PrItem>, batchId: String): EdictNextSignal {
    val item = chunk.firstOrNull { it.workItemId == draft.workItemId } ?: error("Signal work item does not belong to this chunk")
    val diff = draft.diff
    require(diff.before in listOf(item.pr.baseRevision, item.thread.originalCommitSha) && diff.after == item.pr.headRevision) {
      "Diff must compare prepared base/comment revision to head"
    }
    return EdictNextSignal(
      id = draft.id,
      idempotencyKey = draft.idempotencyKey,
      fileRevision = draft.fileRevision,
      source = EdictNextSignalSource.FromPR(
        prNumber = item.pr.number,
        title = item.pr.title,
        discussionMessages = item.thread.messages.map(ReviewMessage::body),
        diffPositiveToNegative = provider.diff(item.repository, diff.before, diff.after, diff.beforePath, diff.afterPath),
        url = item.thread.reviewDiscussionUrl,
      ),
      label = draft.label,
      description = draft.description,
      strength = draft.strength,
      provenance = EdictNextSignalProvenance(draft.workItemId, batchId),
    )
  }

  fun publishValidated(token: String, batchId: String): PrPublicationReceipt = synchronized(store) {
    val batch = batch(token, batchId, coordinator = true)
    val failedChunkIndexes = batch.chunks.indices.filterNot(batch.storedChunks::containsKey)
    val stored = batch.storedChunks.toSortedMap().values
    val validated = stored.flatMap { it.signals.values }
    require(validated.distinctBy(EdictNextSignal::id).size == validated.size) {
      "Duplicate Signal IDs across stored PR chunks"
    }
    val publications = validated.map { store.publishSignal(token, it) }
    val successfulWorkItemCount = stored.sumOf { it.successfulWorkItemIds.size }
    val failedWorkItemCount = batch.items.size - successfulWorkItemCount
    val enoughWorkItemsSucceeded = if (batch.items.isEmpty()) batch.providerProblems.isEmpty()
    else successfulWorkItemCount * 2 > batch.items.size
    val analyzedPrNumbers = batch.selectedPrNumbers.takeIf { batch.analyzedDateRanges.isEmpty() }.orEmpty()
    val coverageUpdated = when {
      batch.analyzedDateRanges.isNotEmpty() -> enoughWorkItemsSucceeded
      analyzedPrNumbers.isNotEmpty() -> failedWorkItemCount == 0 && batch.providerProblems.isEmpty()
      else -> false
    }
    if (coverageUpdated) {
      store.recordPrAnalysisCoverage(
        token,
        RepositoryPrAnalysisCoverage(batch.repository, batch.analyzedDateRanges, analyzedPrNumbers),
      )
    }
    batch.published = true
    val problems = buildList {
      addAll(batch.providerProblems)
      stored.forEach { chunk ->
        addAll(chunk.problems)
        addAll(chunk.failures.map { "${it.workItemId}: ${it.message}" })
      }
      addAll(failedChunkIndexes.map { "Chunk $it did not store a result" })
    }
    PrPublicationReceipt(
      batchId = batchId,
      availableChunkCount = batch.chunks.size,
      storedChunkCount = batch.storedChunks.size,
      failedChunkIndexes = failedChunkIndexes,
      signalIds = publications.map { it.signal.id },
      createdSignalIds = publications.filter { it.created }.map { it.signal.id },
      selectedPrCount = batch.selectedPrCount,
      successfulWorkItemCount = successfulWorkItemCount,
      failedWorkItemCount = failedWorkItemCount,
      analyzedDateRanges = batch.analyzedDateRanges,
      analyzedPrNumbers = analyzedPrNumbers,
      coverageUpdated = coverageUpdated,
      problems = problems,
    )
  }

  fun file(token: String, batchId: String, itemId: String, revision: String, path: String): String {
    val item = get(token, batchId, itemId)
    require(revision in listOf(item.pr.baseRevision, item.pr.headRevision, item.thread.originalCommitSha)) {
      "Revision does not belong to work item"
    }
    return provider.file(item.repository, revision, path)
  }

  fun evidence(item: PrItem): PrItemEvidence {
    val thread = item.thread
    val anchor = thread.anchorLine?.let { it..(thread.anchorEndLine ?: it) }
    val notes = mutableListOf<String>()
    val diff = runCatching {
      provider.diff(item.repository, thread.originalCommitSha, item.pr.headRevision, thread.filePath, thread.filePath, DIFF_CONTEXT)
        .let { diff ->
          val near = anchor?.let { hunksNear(diff, (it.first - DIFF_RADIUS)..(it.last + DIFF_RADIUS)) } ?: diff
          if (near != diff) notes += "the diff shows only changes near the comment; edict_pr_file_diff has the whole file"
          near
        }
        .lines().let { if (it.size <= MAX_DIFF_LINES) it else it.take(MAX_DIFF_LINES) + "… read the full diff for the rest" }
        .joinToString("\n")
    }.onFailure { notes += "diff unavailable: ${it.message}" }.getOrNull()
    val code = anchor?.let { lines ->
      runCatching {
        val content = provider.file(item.repository, thread.originalCommitSha, thread.filePath)
        numberedLines(content, EdictNextLineRange(lines.first, lines.last), CODE_RADIUS)
      }.onFailure { notes += "commented code unavailable: ${it.message}" }.getOrNull()
    }
    return PrItemEvidence(
      path = thread.filePath,
      before = thread.originalCommitSha,
      after = item.pr.headRevision,
      diff = diff?.ifBlank { null } ?: "(no changes to this file near the comment)",
      commentedCode = code,
      notes = notes,
    )
  }

  fun diff(
    token: String,
    batchId: String,
    itemId: String,
    before: String,
    after: String,
    beforePath: String,
    afterPath: String,
  ): String {
    val item = get(token, batchId, itemId)
    require(before in listOf(item.pr.baseRevision, item.thread.originalCommitSha) && after == item.pr.headRevision) {
      "Diff must compare prepared base/comment revision to head"
    }
    return provider.diff(item.repository, before, after, beforePath, afterPath)
  }

  fun complete(token: String) {
    store.requirePrAnalysisCaller(token, coordinator = true)
    require(batches.isNotEmpty()) { "Prepare and inspect requested PR selection before finishing" }
    var totalChunkCount = 0
    var failedChunkCount = 0
    batches.values.forEach { batch ->
      require(batch.published) { "Publish stored PR chunks before finishing" }
      totalChunkCount += batch.chunks.size
      failedChunkCount += batch.chunks.indices.count { it !in batch.storedChunks }
      batch.storedChunks.values.flatMap { it.signals.values }.forEach { signal ->
        val id = signal.id
        require(store.inboxSignal(id) == signal) { "Stored PR Signal is missing or differs from receipt" }
      }
    }
    require(failedChunkCount * 2 <= totalChunkCount) {
      "More than half of PR analysis chunks failed after retry: $failedChunkCount of $totalChunkCount"
    }
  }

  private fun batch(token: String, id: String, coordinator: Boolean): Batch {
    store.requirePrAnalysisCaller(token, coordinator)
    return batches[id] ?: error("Unknown PR batch; prepare again after restart")
  }

  private companion object {
    const val CHUNK_SIZE = 8
    const val MAX_ARTIFACT_BYTES = 8 * 1024 * 1024
    const val CODE_RADIUS = 25
    const val DIFF_CONTEXT = 20
    const val DIFF_RADIUS = 40
    const val MAX_DIFF_LINES = 600
    const val FULL_DATE_FETCH_LIMIT = 1000
    val MIN_REVIEW_DATE: LocalDate = LocalDate.of(1970, 1, 1)
  }
}

private fun RepositoryPrAnalysisCoverage.includes(date: LocalDate): Boolean = analyzedDateRanges.any {
  date in LocalDate.parse(it.startDate)..LocalDate.parse(it.endDate)
}

internal fun hunksNear(diff: String, lines: IntRange): String =
  diff.split(HUNK_START).filterIndexed { index, part ->
    index == 0 || HUNK_BEFORE.find(part)!!.let { header ->
      val start = header.groupValues[1].toInt()
      val end = start + (header.groupValues[2].ifEmpty { "1" }.toInt() - 1).coerceAtLeast(0)
      start <= lines.last && end >= lines.first
    }
  }.joinToString("")

private val HUNK_START = Regex("(?m)^(?=@@ )")
private val HUNK_BEFORE = Regex("""^@@ -(\d+)(?:,(\d+))?""")
