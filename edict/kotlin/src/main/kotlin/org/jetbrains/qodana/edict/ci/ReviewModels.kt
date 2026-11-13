// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci

import kotlinx.serialization.EncodeDefault
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.Transient
import org.jetbrains.qodana.edict.signals.validRevision
import org.jetbrains.qodana.edict.signals.validSourcePath
import java.net.URI
import java.time.LocalDate
import java.time.ZoneOffset

@Serializable
enum class CiProviderId {
  @SerialName("github")
  GITHUB,

  @SerialName("space")
  SPACE,
}

/**
 * A repository of a review provider. [host] is the lowercase host of its web pages, such as `github.com`, which also
 * locates the provider's API; on Space, [owner] is the project key.
 */
@Serializable
data class ReviewRepository(
  val provider: CiProviderId,
  val host: String,
  val owner: String,
  val repository: String,
) {
  init {
    require(host.isNotEmpty() && host == host.lowercase() && host.none { it in "/:@\\ " }) { "Invalid host: $host" }
    require(listOf(owner, repository).all { validSourcePath(it) && '/' !in it }) { "Invalid owner or repository" }
  }

  /** The repository's web page, as `edict.ci.url` names it. */
  fun webUrl(): String = when (provider) {
    CiProviderId.GITHUB -> "https://$host/$owner/$repository"
    CiProviderId.SPACE -> "https://$host/p/$owner/repositories/$repository"
  }

  /** A review of this repository; see [ReviewReference] for what [id] and [url] hold. */
  fun reference(id: String, url: String): ReviewReference = ReviewReference(provider, owner, repository, id, url)

  /** Names the repository in messages; promotion ids hash it, so changing it changes the ids of new promotions. */
  fun label(): String = "${provider.name.lowercase()}:$host/$owner/$repository"

  fun owns(reference: ReviewReference): Boolean = reference.reviewRepository == this

  /** Rejects a review of another repository, which a client of this one cannot reach. */
  fun requireOwns(reference: ReviewReference) {
    require(owns(reference)) { "Review ${reference.url} (${reference.reviewRepository.label()}) is not in the configured ${label()}" }
  }
}

/** Files of a weak-signal source fix, by repository-relative path, applied on top of the full [baseRevision] SHA. */
data class SourceFixChanges(val baseRevision: String, val files: Map<String, String>) {
  init {
    require(validRevision(baseRevision)) { "Source fixes require an exact base revision" }
    require(files.isNotEmpty() && files.keys.all(::validSourcePath)) { "Source fixes require repository-relative file paths" }
  }
}

// TODO: am I correct that both weak signals PR creation and inspection one mixed here? If yes, we need to split them, maybe in later refactoring
/**
 * A review to open against [targetBranch]. [targetPath] is the repository-relative path the inspection is written to,
 * [inspectionDigest] the SHA-256 hex of the cluster's inspection, and [reviewer] the provider login to request. A
 * source-fix request leaves [targetPath] and [inspectionContent] empty.
 */
data class CreateReviewRequest(
  val operationId: String,
  val clusterId: String,
  val targetBranch: String,
  val reviewer: String,
  val targetPath: String,
  val inspectionDigest: String,
  val inspectionContent: String,
  val description: String,
  val sourceFix: SourceFixChanges? = null,
) {
  val branch: String = if (sourceFix == null) "edict/$clusterId-${inspectionDigest.take(12)}" else "edict/weak-$operationId"
  val title: String = if (sourceFix == null) "Add Edict inspection $clusterId" else "Fix issues found by Edict inspection $clusterId"
}

@Serializable
data class ReviewFetchResult(
  val pullRequests: List<PullRequest>,
  /**
   * Why pull requests or threads were left out, one human-readable message each. Pull requests that are not merged, or
   * merged outside the selected dates, are left out without one.
   */
  val problems: List<String> = emptyList(),
  /**
   * Discovery by dates stopped at [ReviewSelection.maxPrs] pull requests, so the dates may hold more. It counts the pull
   * requests found, including those left out with a problem.
   */
  val reachedMaxPrs: Boolean = false,
)

@Serializable
data class ReviewSelection(
  val maxPrs: Int,
  val prNumbers: List<Int> = emptyList(),
  val startDate: String = "",
  val endDate: String = "",
) {
  init {
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
data class ReviewMessage(
  val author: String,
  val body: String,
  /** When the message was posted, as the provider's ISO-8601 instant. */
  val createdAt: String,
  /**
   * Whether a review bot wrote the message: an app, as the provider marks it, since configured bot accounts never reach a
   * thread. What it asks counts only through a change a person made. Written only when true, so that the messages of
   * people, and the batch ids hashed from them, read as before.
   */
  @OptIn(ExperimentalSerializationApi::class)
  @EncodeDefault(EncodeDefault.Mode.NEVER)
  val fromBot: Boolean = false,
)

@Serializable
data class ReviewThread(
  /** `<provider>-<review number>-<root comment id>`: unique among the threads of all fetched reviews. */
  val threadId: String,
  val reviewDiscussionUrl: String,
  val filePath: String,
  val originalCommitSha: String,
  val anchorLine: Int? = null,
  val anchorEndLine: Int? = null,
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
  /** When the review was merged, in epoch milliseconds; on Space, its last change, which for a merged review is its merge. */
  val closeTimestamp: Long,
  val threads: List<ReviewThread> = emptyList(),
)

/** [id] is the pull request number on GitHub and the review id on Space; [url] is the review's web page. */
@Serializable
data class ReviewReference(
  val provider: CiProviderId,
  val owner: String,
  val repository: String,
  val id: String,
  val url: String,
) {
  /** The reviewed repository; its host is the host of [url]. */
  @Transient
  val reviewRepository: ReviewRepository = ReviewRepository(provider, URI(url).host.orEmpty().lowercase(), owner, repository)
}

@Serializable
enum class ReviewState {
  OPEN,
  MERGED,
  CLOSED_UNMERGED,
}

@Serializable
data class ReviewStatus(
  val state: ReviewState,
  /** The reviewers' discussion without bots, from [ReviewApi.reviewWithEvidence]; empty from [ReviewApi.review]. */
  val evidence: String = "",
)

/** Immutable revisions of the change that actually landed, not the proposed PR head. */
@Serializable
data class MergedReview(
  val title: String,
  val discussionMessages: List<String>,
  val beforeRevision: String,
  val mergedRevision: String,
  val changes: List<ReviewFileChange>,
)

@Serializable
data class ReviewFileChange(val beforePath: String, val afterPath: String)
