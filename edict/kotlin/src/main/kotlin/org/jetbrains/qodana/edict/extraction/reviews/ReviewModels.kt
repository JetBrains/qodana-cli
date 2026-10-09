// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.extraction.reviews

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewThread
import java.time.LocalDate

internal const val DAILY_ROUTINE_PROCESSED_PRS = 100

@Serializable
data class PrAnalysisDateRange(val startDate: String, val endDate: String) {
  fun validate() {
    require(!LocalDate.parse(startDate).isAfter(LocalDate.parse(endDate))) {
      "startDate must not follow endDate"
    }
  }
}

@Serializable
data class RepositoryPrAnalysisCoverage(
  val repository: ReviewRepository,
  val analyzedDateRanges: List<PrAnalysisDateRange> = emptyList(),
  val analyzedPrNumbers: List<Int> = emptyList(),
)

@Serializable
data class PrAnalysisCoverageInput(
  val analyzedDateRanges: List<PrAnalysisDateRange> = emptyList(),
  val analyzedPrNumbers: List<Int> = emptyList(),
)

@Serializable
data class ReviewSelectionInput(
  val prNumbers: List<Int> = emptyList(),
  val startDate: String = "",
  val endDate: String = "",
) {
  fun validate() {
    require(prNumbers.all { it > 0 } && prNumbers.distinct().size == prNumbers.size && prNumbers.size <= 1000) {
      "PR numbers must be distinct, positive, and contain at most 1000 entries"
    }
    require((startDate.isEmpty() && endDate.isEmpty()) || (startDate.isNotEmpty() && endDate.isNotEmpty())) {
      "Specify both startDate and endDate, or neither"
    }
    require(prNumbers.isEmpty() || startDate.isEmpty()) { "Select either PR numbers or a date range" }
    if (startDate.isNotEmpty()) PrAnalysisDateRange(startDate, endDate).validate()
  }
}

@Serializable
data class PrAnalysisCoverageState(
  val schemaVersion: Int = 1,
  val repositories: List<RepositoryPrAnalysisCoverage> = emptyList(),
)

@Serializable
data class PrItem(
  val workItemId: String,
  val repository: ReviewRepository,
  val pr: PullRequest,
  val thread: ReviewThread,
  val sourceType: String = "PR_DISCUSSION",
)

@Serializable
data class PrBatchSummary(
  val batchId: String,
  val selectedPrCount: Int,
  val selectedPrNumbers: List<Int>,
  val prCountWithWorkItems: Int,
  val totalWorkItemCount: Int,
  val repository: ReviewRepository,
  val analyzedDateRanges: List<PrAnalysisDateRange> = emptyList(),
  val problems: List<String> = emptyList(),
)

@Serializable
data class PrItemSummary(
  val workItemId: String,
  val prNumber: Int,
  val threadId: String,
  val path: String,
  val messageCount: Int,
)

@Serializable
data class PrPage(
  val totalWorkItemCount: Int,
  val offset: Int,
  val items: List<PrItemSummary>,
  val nextOffset: Int? = null,
)

@Serializable
data class PrReceipt(
  val batchId: String,
  val inspectedWorkItemCount: Int,
  val signalCount: Int,
  val signalIds: List<String>,
)

@Serializable
data class PrPublicationReceipt(
  val batchId: String,
  val signalIds: List<String>,
  val createdSignalIds: List<String>,
)
