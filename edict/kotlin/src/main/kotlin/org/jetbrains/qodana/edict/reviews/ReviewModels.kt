// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.reviews

import java.time.LocalDate
import java.time.ZoneOffset
import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.signals.validSourcePath

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
