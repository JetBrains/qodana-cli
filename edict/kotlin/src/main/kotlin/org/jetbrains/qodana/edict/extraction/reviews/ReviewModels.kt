// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.extraction.reviews

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewThread
import org.jetbrains.qodana.edict.core.EdictFileRevision
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import org.jetbrains.qodana.edict.core.EdictSignalStrength
import java.time.LocalDate

@Serializable
data class PrItemEvidence(
  val path: String,
  val before: String,
  val after: String,
  val diff: String,
  val commentedCode: String? = null,
  val notes: List<String> = emptyList(),
)

@Serializable
data class PrSignalDiff(
  val before: String,
  val after: String,
  val beforePath: String,
  val afterPath: String,
  val relatedBeforeRanges: List<EdictLineRange> = emptyList(),
  val relatedAfterRanges: List<EdictLineRange> = emptyList(),
)

@Serializable
data class PrSignalDraft(
  val workItemId: String,
  val fileRevision: EdictFileRevision,
  val label: EdictSignalLabel,
  val description: String,
  val strength: EdictSignalStrength = EdictSignalStrength.STRONG,
  val diff: PrSignalDiff,
  /** Shared by the POSITIVE and NEGATIVE drafts of one correction in a work item; not stored in the Signal. */
  val correction: Int? = null,
)

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
    require((startDate.isEmpty() && endDate.isEmpty()) || (startDate.isNotEmpty() && endDate.isNotEmpty())) {
      "Specify both startDate and endDate, or neither"
    }
    require(prNumbers.isEmpty() || startDate.isEmpty()) { "Select either PR numbers or a date range" }
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
  val availableChunkCount: Int,
)

@Serializable
data class PrChunk(
  val batchId: String,
  val chunkIndex: Int,
  val workItemIds: List<String>,
)

@Serializable
data class PrReceipt(
  val batchId: String,
  val chunkIndex: Int,
  val inspectedWorkItemCount: Int,
  val successfulWorkItemCount: Int,
  val failedWorkItemCount: Int,
  val signalCount: Int,
  val signalIds: List<String>,
  val failures: List<PrSignalStoreFailure> = emptyList(),
  val problems: List<String> = emptyList(),
  val unpairedCorrections: List<String> = emptyList(),
)

@Serializable
data class PrSignalStoreFailure(
  val signalIndex: Int,
  val signalId: String,
  val workItemId: String,
  val message: String,
)

@Serializable
data class PrPublicationReceipt(
  val batchId: String,
  val availableChunkCount: Int,
  val storedChunkCount: Int,
  val failedChunkIndexes: List<Int>,
  val signalIds: List<String>,
  val createdSignalIds: List<String>,
  val selectedPrCount: Int,
  val successfulWorkItemCount: Int,
  val failedWorkItemCount: Int,
  val analyzedDateRanges: List<PrAnalysisDateRange> = emptyList(),
  val analyzedPrNumbers: List<Int> = emptyList(),
  val coverageUpdated: Boolean,
  val problems: List<String> = emptyList(),
)
