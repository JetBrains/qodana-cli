// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.reviews

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.model.Signal
import org.jetbrains.qodana.edict.model.SignalLabel
import org.jetbrains.qodana.edict.signals.SignalValidation
import org.jetbrains.qodana.edict.store.MAX_ARTIFACT_BYTES
import org.jetbrains.qodana.edict.store.Store

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
