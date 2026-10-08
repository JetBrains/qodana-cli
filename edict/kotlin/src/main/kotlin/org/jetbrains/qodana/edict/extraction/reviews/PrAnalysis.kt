// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.extraction.reviews

import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewExtractionApi
import org.jetbrains.qodana.edict.ci.ReviewMessage
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.signals.SignalValidation
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.time.temporal.ChronoUnit

/** Prepared PR evidence and validation receipts scoped to one managed PR-analysis task. */
internal class PrAnalysis(
  private val store: EdictNextRepositoryState,
  private val provider: ReviewExtractionApi,
  private val dailyProcessedPrTarget: Int = DAILY_ROUTINE_PROCESSED_PRS,
  private val today: () -> LocalDate = { LocalDate.now(ZoneOffset.UTC) },
) {
  private data class Batch(
    val summary: PrBatchSummary,
    val items: List<PrItem>,
    var validated: Map<String, EdictNextSignal>? = null,
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
   * Selects complete uncovered UTC dates, newest first, until enough PRs with analyzable review work are present.
   * The current UTC date is never selected because it is not complete yet.
   */
  private fun prepareDefaultBatch(token: String, repository: ReviewRepository): PrBatchSummary {
    require(dailyProcessedPrTarget >= 1) { "Daily processed PR target must be positive" }
    val coverage = store.getPrAnalysisCoverage(token, repository)
    val selectedDates = mutableListOf<LocalDate>()
    val selectedPrs = mutableListOf<PullRequest>()
    var relevantPrCount = 0
    var rangeEnd = today().minusDays(1)
    var windowDays = 1L
    while (!rangeEnd.isBefore(MIN_REVIEW_DATE)) {
      val rangeStart = maxOf(MIN_REVIEW_DATE, rangeEnd.minusDays(windowDays - 1))
      val prs = fetchCompleteRange(repository, rangeStart, rangeEnd)
      val prsByDate = prs.groupBy { it.utcCloseDate() }
      var date = rangeEnd
      while (!date.isBefore(rangeStart)) {
        if (!coverage.includes(date)) {
          val datePrs = prsByDate[date].orEmpty()
            .filterNot { it.number in coverage.analyzedPrNumbers }
            .sortedByDescending(PullRequest::closeTimestamp)
          selectedDates += date
          selectedPrs += datePrs
          relevantPrCount += datePrs.count { it.threads.isNotEmpty() }
          if (relevantPrCount >= dailyProcessedPrTarget) break
        }
        date = date.minusDays(1)
      }
      if (relevantPrCount >= dailyProcessedPrTarget || rangeStart == MIN_REVIEW_DATE) break
      rangeEnd = rangeStart.minusDays(1)
      windowDays = (windowDays * 2).coerceAtMost(MAX_DATE_WINDOW_DAYS)
    }
    val ranges = selectedDates.toRanges()
    return registerBatch(
      token,
      repository,
      selectedPrs,
      json.encodeToString(repository) + json.encodeToString(ranges),
      ranges,
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
    val prs = fetchCompleteRange(repository, start, end)
    val ranges = listOf(PrAnalysisDateRange(startDate, endDate))
    return registerBatch(
      token,
      repository,
      prs.sortedByDescending(PullRequest::closeTimestamp),
      json.encodeToString(repository) + json.encodeToString(ranges),
      ranges,
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
    return registerBatch(token, repository, provider.fetch(selection), json.encodeToString(selection))
  }

  private fun fetchCompleteRange(
    repository: ReviewRepository,
    startDate: LocalDate,
    endDate: LocalDate,
  ): List<PullRequest> {
    val selection = ReviewSelection(
      repository.provider,
      repository.owner,
      repository.repository,
      FULL_DATE_FETCH_LIMIT,
      startDate = startDate.toString(),
      endDate = endDate.toString(),
    )
    val prs = provider.fetch(selection)
    require(prs.all { selection.containsDate(it.closeTimestamp) }) { "Provider returned a PR outside the requested dates" }
    if (prs.size < FULL_DATE_FETCH_LIMIT) return prs
    require(startDate != endDate) {
      "Cannot prove that UTC date $startDate was fetched completely: it contains at least $FULL_DATE_FETCH_LIMIT PRs"
    }
    val midpoint = startDate.plusDays(ChronoUnit.DAYS.between(startDate, endDate) / 2)
    return fetchCompleteRange(repository, midpoint.plusDays(1), endDate) +
      fetchCompleteRange(repository, startDate, midpoint)
  }

  private fun registerBatch(
    token: String,
    repository: ReviewRepository,
    prs: List<PullRequest>,
    selectionIdentity: String,
    analyzedDateRanges: List<PrAnalysisDateRange> = emptyList(),
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
    val id = sha256(selectionIdentity + json.encodeToString(prs)).take(24)
    val summary = PrBatchSummary(
      id,
      prs.size,
      prs.map(PullRequest::number),
      prs.count { it.threads.isNotEmpty() },
      items.size,
      repository,
      analyzedDateRanges,
    )
    return synchronized(store) {
      store.requirePrAnalysisCaller(token, coordinator = true)
      batches.getOrPut(id) { Batch(summary, items) }.summary
    }
  }

  fun list(token: String, batchId: String, offset: Int, limit: Int): PrPage = synchronized(store) {
    val batch = batch(token, batchId, coordinator = false)
    require(offset in 0..batch.items.size && limit in 1..20) {
      "offset must be within batch and limit must be 1..20"
    }
    val end = minOf(offset + limit, batch.items.size)
    PrPage(
      batch.items.size,
      offset,
      batch.items.subList(offset, end).map {
        PrItemSummary(it.workItemId, it.pr.number, it.thread.threadId, it.thread.filePath, it.thread.messages.size)
      },
      end.takeIf { it < batch.items.size },
    )
  }

  fun get(token: String, batchId: String, itemId: String): PrItem = synchronized(store) {
    batch(token, batchId, coordinator = false).items.firstOrNull { it.workItemId == itemId }
      ?: error("Unknown work item in PR batch")
  }

  fun validate(
    token: String,
    batchId: String,
    inspected: List<String>,
    signals: List<EdictNextSignal>,
  ): PrReceipt = synchronized(store) {
    val batch = batch(token, batchId, coordinator = true)
    require(inspected == batch.items.map(PrItem::workItemId)) { "Incomplete or unordered PR coverage" }
    val validated = linkedMapOf<String, EdictNextSignal>()
    signals.forEach { candidate ->
      require(wireJson.encodeToString(candidate).toByteArray().size <= MAX_ARTIFACT_BYTES) { "Signal exceeds 8 MiB" }
      val signal = SignalValidation.validate(candidate)
      val item = batch.items.firstOrNull { it.workItemId == signal.provenance.workItemId }
        ?: error("Unknown signal work item")
      require(signal.id !in validated) { "Duplicate signal" }
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
      validated[signal.id] = signal
    }
    batch.validated = validated
    PrReceipt(batchId, inspected.size, validated.size, validated.keys.toList())
  }

  fun publishValidated(token: String, batchId: String): PrPublicationReceipt = synchronized(store) {
    val batch = batch(token, batchId, coordinator = true)
    val validated = checkNotNull(batch.validated) { "Validate complete PR inspection coverage before publication" }
    val publications = validated.values.map { store.publishSignal(token, it) }
    if (batch.summary.analyzedDateRanges.isNotEmpty()) {
      store.recordPrAnalysisCoverage(
        token,
        RepositoryPrAnalysisCoverage(batch.summary.repository, batch.summary.analyzedDateRanges),
      )
    }
    PrPublicationReceipt(
      batchId = batchId,
      signalIds = publications.map { it.signal.id },
      createdSignalIds = publications.filter { it.created }.map { it.signal.id },
    )
  }

  fun file(token: String, batchId: String, itemId: String, revision: String, path: String): String {
    val item = get(token, batchId, itemId)
    require(revision in listOf(item.pr.baseRevision, item.pr.headRevision, item.thread.originalCommitSha)) {
      "Revision does not belong to work item"
    }
    return provider.file(item.repository, revision, path)
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
    batches.values.forEach { batch ->
      val validated = checkNotNull(batch.validated) { "Validate complete PR inspection coverage before finishing" }
      validated.forEach { (id, signal) ->
        require(store.inboxSignal(id) == signal) { "Validated PR Signal is missing or differs from receipt" }
      }
    }
  }

  private fun batch(token: String, id: String, coordinator: Boolean): Batch {
    store.requirePrAnalysisCaller(token, coordinator)
    return batches[id] ?: error("Unknown PR batch; prepare again after restart")
  }

  private companion object {
    const val MAX_ARTIFACT_BYTES = 8 * 1024 * 1024
    const val FULL_DATE_FETCH_LIMIT = 1000
    const val MAX_DATE_WINDOW_DAYS = 16_384L
    val MIN_REVIEW_DATE: LocalDate = LocalDate.of(1970, 1, 1)
  }
}

private fun PullRequest.utcCloseDate(): LocalDate =
  Instant.ofEpochMilli(closeTimestamp).atZone(ZoneOffset.UTC).toLocalDate()

private fun RepositoryPrAnalysisCoverage.includes(date: LocalDate): Boolean = analyzedDateRanges.any {
  date in LocalDate.parse(it.startDate)..LocalDate.parse(it.endDate)
}

private fun List<LocalDate>.toRanges(): List<PrAnalysisDateRange> =
  sorted().fold(mutableListOf<Pair<LocalDate, LocalDate>>()) { ranges, date ->
    val previous = ranges.lastOrNull()
    if (previous != null && date == previous.second.plusDays(1)) ranges[ranges.lastIndex] = previous.first to date
    else ranges += date to date
    ranges
  }.map { (start, end) -> PrAnalysisDateRange(start.toString(), end.toString()) }
