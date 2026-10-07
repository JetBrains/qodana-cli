// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci

import kotlinx.serialization.json.JsonPrimitive
import org.jetbrains.qodana.edict.ci.github.GitHubReviewApi
import org.jetbrains.qodana.edict.ci.space.SpaceReviewApi
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.signals.validRevision
import org.jetbrains.qodana.edict.signals.validSourcePath
import java.nio.file.Files
import java.nio.file.NoSuchFileException
import java.util.Comparator

data class CreateReviewRequest(
  val operationId: String,
  val clusterId: String,
  val repository: ReviewRepository,
  val targetBranch: String,
  val reviewer: String,
  val targetPath: String,
  val inspectionDigest: String,
  val inspectionContent: ByteArray,
) {
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (javaClass != other?.javaClass) return false

        other as CreateReviewRequest

        if (operationId != other.operationId) return false
        if (clusterId != other.clusterId) return false
        if (repository != other.repository) return false
        if (targetBranch != other.targetBranch) return false
        if (reviewer != other.reviewer) return false
        if (targetPath != other.targetPath) return false
        if (inspectionDigest != other.inspectionDigest) return false
        if (!inspectionContent.contentEquals(other.inspectionContent)) return false

        return true
    }

    override fun hashCode(): Int {
        var result = operationId.hashCode()
        result = 31 * result + clusterId.hashCode()
        result = 31 * result + repository.hashCode()
        result = 31 * result + targetBranch.hashCode()
        result = 31 * result + reviewer.hashCode()
        result = 31 * result + targetPath.hashCode()
        result = 31 * result + inspectionDigest.hashCode()
        result = 31 * result + inspectionContent.contentHashCode()
        return result
    }
}

interface ReviewExtractionApi {
  fun fetch(selection: ReviewSelection): List<PullRequest>
  fun file(repository: ReviewRepository, revision: String, path: String): String
  fun diff(repository: ReviewRepository, before: String, after: String, beforePath: String, afterPath: String): String
}

interface ReviewManagementApi {
  fun resolveTargetBranch(repository: ReviewRepository, configured: String?): String
  fun ensureReview(request: CreateReviewRequest): CreatedReview
  fun review(reference: ReviewReference): ReviewStatus
}

internal interface ProviderReviewApi {
  val id: CiProviderId
  fun fetch(selection: ReviewSelection): List<PullRequest>
  fun file(repository: ReviewRepository, revision: String, path: String): String
  fun resolveTargetBranch(repository: ReviewRepository, configured: String?): String
  fun ensureReview(request: CreateReviewRequest): CreatedReview
  fun review(reference: ReviewReference): ReviewStatus
}

/** One CI review API shared by extraction and promotion. */
class ReviewClient private constructor(
  providers: List<ProviderReviewApi>,
) : ReviewExtractionApi, ReviewManagementApi {
  private val providers = providers.associateBy(ProviderReviewApi::id).also {
    require(it.keys == CiProviderId.entries.toSet()) { "Review APIs must be configured for every CI provider" }
  }

  constructor(
    githubUrl: String = System.getenv("EDICT_GITHUB_API_URL") ?: "https://api.github.com",
    spaceUrl: String = System.getenv("EDICT_SPACE_URL") ?: "https://jetbrains.team",
    githubToken: String = System.getenv("GITHUB_TOKEN") ?: System.getenv("GH_TOKEN").orEmpty(),
    spaceToken: String = System.getenv("SPACE_TOKEN").orEmpty(),
  ) : this(
    DefaultCiHttpTransport(githubUrl, spaceUrl, githubToken, spaceToken).let { transport ->
      listOf(GitHubReviewApi(transport), SpaceReviewApi(transport, spaceUrl))
    },
  )

  override fun fetch(selection: ReviewSelection): List<PullRequest> {
    selection.validate()
    val reviews = provider(selection.provider).fetch(selection)
    reviews.forEach { pullRequest ->
      require(validRevision(pullRequest.baseRevision) && validRevision(pullRequest.headRevision)) {
        "PR ${pullRequest.number} lacks exact base/head revisions"
      }
      pullRequest.threads.forEach { thread ->
        require(
          validSourcePath(thread.filePath) && validRevision(thread.originalCommitSha) &&
            thread.anchorLine > 0 && thread.anchorEndLine >= thread.anchorLine,
        ) { "Invalid discussion path, revision or anchors" }
      }
    }
    return if (selection.prNumbers.isEmpty()) {
      reviews.sortedWith(compareBy(PullRequest::closeTimestamp, PullRequest::number))
    }
    else reviews
  }

  override fun file(repository: ReviewRepository, revision: String, path: String): String {
    require(validRevision(revision) && validSourcePath(path)) {
      "Source reads require exact revision and repository-relative path"
    }
    return provider(repository.provider).file(repository, revision, path)
  }

  override fun diff(
    repository: ReviewRepository,
    before: String,
    after: String,
    beforePath: String,
    afterPath: String,
  ): String {
    fun readOrMissing(revision: String, path: String): String? = try {
      file(repository, revision, path)
    }
    catch (_: NoSuchFileException) {
      null
    }

    val old = readOrMissing(before, beforePath)
    val new = readOrMissing(after, afterPath)
    require(old != null || new != null) { "File absent at both revisions" }
    val scratch = Files.createTempDirectory("edict-review-diff-")
    try {
      runProcess(scratch, listOf("git", "init", "-q"))
      Files.writeString(scratch.resolve("source"), old.orEmpty())
      runProcess(scratch, listOf("git", "add", "source"))
      Files.writeString(scratch.resolve("source"), new.orEmpty())
      val diff = runProcess(
        scratch,
        listOf(
          "git", "--no-pager", "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--unified=200", "--", "source",
        ),
      )
      if (diff.isEmpty()) return ""
      val index = diff.indexOf("@@ ")
      require(index >= 0) { "Provider snapshot diff has no source hunks" }
      fun quoted(path: String): String = if (path.any { it in "\t\"\\" }) JsonPrimitive(path).toString() else path
      return "--- ${quoted(if (old == null) "/dev/null" else "a/$beforePath")}\n" +
        "+++ ${quoted(if (new == null) "/dev/null" else "b/$afterPath")}\n${diff.substring(index)}"
    }
    finally {
      Files.walk(scratch).use { paths -> paths.sorted(Comparator.reverseOrder()).forEach(Files::delete) }
    }
  }

  override fun resolveTargetBranch(repository: ReviewRepository, configured: String?): String =
    provider(repository.provider).resolveTargetBranch(repository, configured)

  override fun ensureReview(request: CreateReviewRequest): CreatedReview =
    provider(request.repository.provider).ensureReview(request)

  override fun review(reference: ReviewReference): ReviewStatus = provider(reference.provider).review(reference)

  private fun provider(id: CiProviderId): ProviderReviewApi = requireNotNull(providers[id])
}
