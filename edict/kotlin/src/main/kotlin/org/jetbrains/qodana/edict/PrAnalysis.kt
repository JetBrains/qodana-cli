// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*
import java.time.LocalDate
import java.time.ZoneOffset

@Serializable data class ReviewRepository(val provider: String, val owner: String, val repo: String)
@Serializable data class ReviewSelection(
    val provider: String, val owner: String, val repo: String, val maxPrs: Int,
    val prNumbers: List<Int> = emptyList(), val startDate: String = "", val endDate: String = "",
) {
    val repository: ReviewRepository get() = ReviewRepository(provider, owner, repo)
    fun validate() {
        require(provider in listOf("github", "space")) { "Provider must be github or space" }
        require(listOf(owner, repo).all { validSourcePath(it) && '/' !in it }) { "Invalid owner or repo" }
        require(maxPrs in 1..1000) { "maxPrs must be 1..1000" }
        if (prNumbers.isNotEmpty()) require(startDate.isEmpty() && endDate.isEmpty() && prNumbers.all { it > 0 } &&
            prNumbers.distinct().size == prNumbers.size && prNumbers.size <= maxPrs) { "Require distinct positive PR numbers within maxPrs, or dates" }
        else require(!LocalDate.parse(startDate).isAfter(LocalDate.parse(endDate))) { "startDate must not follow endDate" }
    }
    fun containsDate(timestamp: Long): Boolean = prNumbers.isNotEmpty() ||
        timestamp >= LocalDate.parse(startDate).atStartOfDay().toInstant(ZoneOffset.UTC).toEpochMilli() &&
        timestamp < LocalDate.parse(endDate).plusDays(1).atStartOfDay().toInstant(ZoneOffset.UTC).toEpochMilli()
}
@Serializable data class ReviewMessage(val author: String, val body: String, val createdAt: String)
@Serializable data class ReviewThread(val threadId: String, val reviewDiscussionUrl: String, val filePath: String,
    val originalCommitSha: String, val anchorLine: Int, val anchorEndLine: Int, val messages: List<ReviewMessage>)
@Serializable data class PullRequest(val number: Int, val url: String, val title: String, val body: String,
    val baseRevision: String, val headRevision: String, val closeTimestamp: Long, val threads: List<ReviewThread> = emptyList())
interface ReviewProvider {
    fun fetch(selection: ReviewSelection): List<PullRequest>
    fun file(repository: ReviewRepository, revision: String, path: String): String
    fun diff(repository: ReviewRepository, before: String, after: String, beforePath: String, afterPath: String): String
}
@Serializable data class PrItem(val workItemId: String, val repository: ReviewRepository, val pr: PullRequest, val thread: ReviewThread, val sourceType: String = "PR_DISCUSSION")
@Serializable data class PrBatchSummary(val batchId: String, val selectedPrCount: Int, val prCountWithWorkItems: Int, val totalWorkItemCount: Int)
@Serializable data class PrItemSummary(val workItemId: String, val prNumber: Int, val threadId: String, val path: String, val messageCount: Int)
@Serializable data class PrPage(val totalWorkItemCount: Int, val offset: Int, val items: List<PrItemSummary>, val nextOffset: Int? = null)
@Serializable data class PrReceipt(val batchId: String, val inspectedWorkItemCount: Int, val signalCount: Int, val signals: Map<String, String>)

/** Mirrors PREdict's prepared work-item coverage and managed Go's task-bound validation receipts. */
class PrAnalysis(private val store: Store, private val provider: ReviewProvider) {
    private data class Batch(val owner: String, val summary: PrBatchSummary, val items: List<PrItem>, var validated: Map<String, String>? = null)
    private val batches = mutableMapOf<String, Batch>()
    init { store.prAnalysis = this }

    fun prepare(token: String, selection: ReviewSelection): PrBatchSummary {
        synchronized(store) { owner(token, true) }
        selection.validate()
        val prs = provider.fetch(selection)
        val items = prs.flatMap { pr -> pr.threads.map { thread ->
            val identity = buildJsonArray {
                add(wireJson.encodeToJsonElement(selection.repository)); add(pr.url); add(pr.number)
                add(thread.threadId); add(thread.filePath); add(thread.anchorLine); add(thread.anchorEndLine)
            }
            val id = "pr-${pr.number}-${sha256(wireJson.encodeToString(identity)).take(16)}"
            PrItem(id, selection.repository, pr.copy(threads = emptyList()), thread)
        } }
        require(items.distinctBy { it.workItemId }.size == items.size) { "Duplicate provider work item" }
        val id = sha256(json.encodeToString(selection) + json.encodeToString(prs)).take(24)
        val summary = PrBatchSummary(id, prs.size, prs.count { it.threads.isNotEmpty() }, items.size)
        return synchronized(store) {
            val owner = owner(token, true)
            batches.getOrPut("$owner:$id") { Batch(owner, summary, items) }.summary
        }
    }
    fun list(token: String, batchId: String, offset: Int, limit: Int): PrPage = synchronized(store) {
        val batch = batch(token, batchId, false)
        require(offset in 0..batch.items.size && limit in 1..20) { "offset must be within batch and limit must be 1..20" }
        val end = minOf(offset + limit, batch.items.size)
        PrPage(batch.items.size, offset, batch.items.subList(offset, end).map {
            PrItemSummary(it.workItemId, it.pr.number, it.thread.threadId, it.thread.filePath, it.thread.messages.size)
        }, end.takeIf { it < batch.items.size })
    }
    fun get(token: String, batchId: String, itemId: String): PrItem = synchronized(store) {
        batch(token, batchId, false).items.firstOrNull { it.workItemId == itemId } ?: error("Unknown work item in PR batch")
    }
    fun validate(token: String, batchId: String, inspected: List<String>, contents: List<String>): PrReceipt = synchronized(store) {
        val batch = batch(token, batchId, true)
        require(inspected == batch.items.map { it.workItemId }) { "Incomplete or unordered PR coverage" }
        val validated = linkedMapOf<String, String>()
        contents.forEach { content ->
            require(content.toByteArray().size <= MAX_ARTIFACT_BYTES) { "Signal exceeds 8 MiB" }
            val candidate = json.decodeFromString<Signal>(content)
            val signal = SignalValidation.validate("inbox/${candidate.id}.json", content)
            val item = batch.items.firstOrNull { it.workItemId == signal.provenance.workItemId } ?: error("Unknown signal work item")
            require(signal.id !in validated) { "Duplicate signal" }
            require(signal.source.type == "FromPR" && signal.source.prNumber == item.pr.number && signal.source.title == item.pr.title &&
                signal.source.url == item.thread.reviewDiscussionUrl) { "Signal source must preserve prepared PR number, title and discussion URL" }
            require(signal.source.discussionMessages == item.thread.messages.map { it.body }) { "Preserve all prepared discussion messages in order" }
            require(if (signal.label == SignalLabel.NEGATIVE) signal.fileRevision.revision == item.pr.headRevision
                else signal.fileRevision.revision in listOf(item.pr.baseRevision, item.thread.originalCommitSha)) { "Evidence revision does not match its PR side" }
            validated[signal.id] = sha256(content)
        }
        batch.validated = validated
        PrReceipt(batchId, inspected.size, validated.size, validated.mapKeys { "inbox/${it.key}.json" })
    }
    fun file(token: String, batchId: String, itemId: String, revision: String, path: String): String {
        val item = get(token, batchId, itemId)
        require(revision in listOf(item.pr.baseRevision, item.pr.headRevision, item.thread.originalCommitSha)) { "Revision does not belong to work item" }
        return provider.file(item.repository, revision, path)
    }
    fun diff(token: String, batchId: String, itemId: String, before: String, after: String, beforePath: String, afterPath: String): String {
        val item = get(token, batchId, itemId)
        require(before in listOf(item.pr.baseRevision, item.thread.originalCommitSha) && after == item.pr.headRevision) { "Diff must compare prepared base/comment revision to head" }
        return provider.diff(item.repository, before, after, beforePath, afterPath)
    }
    internal fun validateWrite(owner: String, id: String, content: String) {
        require(batches.values.any { it.owner == owner && it.validated?.get(id) == sha256(content) }) { "PR signal has not passed edict_validate_pr_signals with these exact bytes" }
    }
    internal fun complete(owner: String) {
        val selected = batches.values.filter { it.owner == owner }
        require(selected.isNotEmpty()) { "Prepare and inspect requested PR selection before finishing" }
        selected.forEach { batch ->
            val validated = checkNotNull(batch.validated) { "Validate complete PR inspection coverage before finishing" }
            validated.forEach { (id, hash) -> require(store.read("inbox/$id.json").hash == hash) { "Validated PR signal is missing or differs from receipt" } }
        }
    }
    private fun owner(token: String, coordinator: Boolean): String {
        val c = store.authorize(token)
        if (c.skill == "edict-pr-signal-analysis") return c.taskId
        if (!coordinator && c.skill == "edict-signal-analysis") {
            val parent = store.task(store.task(c.taskId).parentId)
            if (parent.skill == "edict-pr-signal-analysis") return parent.id
        }
        error("PR analysis requires a running PR-analysis task${if (coordinator) "" else " or its signal-analysis worker"}")
    }
    private fun batch(token: String, id: String, coordinator: Boolean): Batch =
        batches["${owner(token, coordinator)}:$id"] ?: error("Unknown PR batch for this task; prepare again after restart")
}
