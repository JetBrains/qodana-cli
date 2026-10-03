// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.extraction.reviews

import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.signals.SignalValidation

/** Prepared PR evidence and validation receipts scoped to one managed PR-analysis task. */
internal class PrAnalysis(
  private val store: EdictNextRepositoryState,
  private val provider: ReviewProvider,
) {
  private data class Batch(
    val owner: String,
    val summary: PrBatchSummary,
    val items: List<PrItem>,
    var validated: Map<String, String>? = null,
  )

  private val batches = mutableMapOf<String, Batch>()

  fun prepare(token: String, selection: ReviewSelection): PrBatchSummary {
    store.prAnalysisOwner(token, coordinator = true)
    selection.validate()
    val prs = provider.fetch(selection)
    val items = prs.flatMap { pr ->
      pr.threads.map { thread ->
        val identity = buildJsonArray {
          add(wireJson.encodeToJsonElement(selection.repository))
          add(pr.url)
          add(pr.number)
          add(thread.threadId)
          add(thread.filePath)
          add(thread.anchorLine)
          add(thread.anchorEndLine)
        }
        val id = "pr-${pr.number}-${sha256(wireJson.encodeToString(identity)).take(16)}"
        PrItem(id, selection.repository, pr.copy(threads = emptyList()), thread)
      }
    }
    require(items.distinctBy(PrItem::workItemId).size == items.size) { "Duplicate provider work item" }
    val id = sha256(json.encodeToString(selection) + json.encodeToString(prs)).take(24)
    val summary = PrBatchSummary(id, prs.size, prs.count { it.threads.isNotEmpty() }, items.size)
    return synchronized(store) {
      val owner = store.prAnalysisOwner(token, coordinator = true)
      batches.getOrPut("$owner:$id") { Batch(owner, summary, items) }.summary
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
    contents: List<String>,
  ): PrReceipt = synchronized(store) {
    val batch = batch(token, batchId, coordinator = true)
    require(inspected == batch.items.map(PrItem::workItemId)) { "Incomplete or unordered PR coverage" }
    val validated = linkedMapOf<String, String>()
    contents.forEach { content ->
      require(content.toByteArray().size <= MAX_ARTIFACT_BYTES) { "Signal exceeds 8 MiB" }
      val candidate = json.decodeFromString<EdictNextSignal>(content)
      val signal = SignalValidation.validate("inbox/${candidate.id}.json", content)
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
      validated[signal.id] = sha256(content)
    }
    batch.validated = validated
    PrReceipt(batchId, inspected.size, validated.size, validated.mapKeys { "inbox/${it.key}.json" })
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

  fun validateWrite(token: String, id: String, content: String) {
    val owner = store.prAnalysisOwner(token, coordinator = true)
    require(batches.values.any { it.owner == owner && it.validated?.get(id) == sha256(content) }) {
      "PR signal has not passed edict_validate_pr_signals with these exact bytes"
    }
  }

  fun complete(token: String) {
    val owner = store.prAnalysisOwner(token, coordinator = true)
    val selected = batches.values.filter { it.owner == owner }
    require(selected.isNotEmpty()) { "Prepare and inspect requested PR selection before finishing" }
    selected.forEach { batch ->
      val validated = checkNotNull(batch.validated) { "Validate complete PR inspection coverage before finishing" }
      validated.forEach { (id, hash) ->
        require(store.inboxSignalHash(id) == hash) { "Validated PR signal is missing or differs from receipt" }
      }
    }
  }

  private fun batch(token: String, id: String, coordinator: Boolean): Batch {
    val owner = store.prAnalysisOwner(token, coordinator)
    return batches["$owner:$id"] ?: error("Unknown PR batch for this task; prepare again after restart")
  }

  private companion object {
    const val MAX_ARTIFACT_BYTES = 8 * 1024 * 1024
  }
}
