// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.signals.validSourcePath
import java.time.LocalDate
import java.time.ZoneOffset

@Serializable
enum class CiProviderId {
  @SerialName("github")
  GITHUB,

  @SerialName("space")
  SPACE,
}

@Serializable
data class ReviewRepository(
  val provider: CiProviderId,
  val owner: String,
  @SerialName("repo") val repository: String,
) {
  fun validate() {
    require(listOf(owner, repository).all { validSourcePath(it) && '/' !in it }) { "Invalid owner or repository" }
  }
}

@Serializable
data class ReviewSelection(
  val provider: CiProviderId,
  val owner: String,
  @SerialName("repo") val repository: String,
  val maxPrs: Int,
  val prNumbers: List<Int> = emptyList(),
  val startDate: String = "",
  val endDate: String = "",
) {
  val repositoryRef: ReviewRepository get() = ReviewRepository(provider, owner, repository)

  fun validate() {
    repositoryRef.validate()
    require(maxPrs in 1..1000) { "maxPrs must be 1..1000" }
    if (prNumbers.isNotEmpty()) {
      require(
        startDate.isEmpty() && endDate.isEmpty() && prNumbers.all { it > 0 } &&
          prNumbers.distinct().size == prNumbers.size && prNumbers.size <= maxPrs,
      ) { "Require distinct positive PR numbers within maxPrs, or dates" }
    }
    else {
      require(!LocalDate.parse(startDate).isAfter(LocalDate.parse(endDate))) {
        "startDate must not follow endDate"
      }
    }
  }

  fun containsDate(timestamp: Long): Boolean = prNumbers.isNotEmpty() ||
    timestamp >= LocalDate.parse(startDate).atStartOfDay().toInstant(ZoneOffset.UTC).toEpochMilli() &&
    timestamp < LocalDate.parse(endDate).plusDays(1).atStartOfDay().toInstant(ZoneOffset.UTC).toEpochMilli()
}

@Serializable
data class ReviewMessage(val author: String, val body: String, val createdAt: String)

@Serializable
data class ReviewThread(
  val threadId: String,
  val reviewDiscussionUrl: String,
  val filePath: String,
  val originalCommitSha: String,
  val anchorLine: Int,
  val anchorEndLine: Int,
  val messages: List<ReviewMessage>,
)

@Serializable
data class PullRequest(
  val number: Int,
  val url: String,
  val title: String,
  val body: String,
  val baseRevision: String,
  val headRevision: String,
  val closeTimestamp: Long,
  val threads: List<ReviewThread> = emptyList(),
)

@Serializable
data class ReviewReference(
  val provider: CiProviderId,
  val owner: String,
  val repository: String,
  val id: String,
  val url: String,
)

enum class ReviewState {
  OPEN,
  MERGED,
  CLOSED_UNMERGED,
}

data class ReviewStatus(
  val state: ReviewState,
  val evidence: String = "",
)

data class CreatedReview(
  val id: String,
  val url: String,
)
