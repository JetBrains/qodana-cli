// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci

import org.jetbrains.qodana.edict.ci.github.gitHubReviewApi
import org.jetbrains.qodana.edict.ci.space.spaceReviewApi
import org.jetbrains.qodana.edict.signals.UnifiedDiff
import java.nio.file.NoSuchFileException

/** Reads and opens reviews of one [repository]. Revisions are full commit SHAs; paths are repository-relative. */
interface ReviewApi {
  val repository: ReviewRepository
  fun fetch(selection: ReviewSelection): ReviewFetchResult

  /**
   * The file's text at [revision]; throws [NoSuchFileException] when it doesn't exist there, and fails when the
   * revision doesn't exist.
   */
  fun file(revision: String, path: String): String

  /** The branch to open reviews against: the [configured] branch name, or the repository's default branch when null. */
  fun resolveTargetBranch(configured: String?): String

  /**
   * Pushes the [request]'s branch and returns its review: the one already opened from that branch into its target
   * branch, in any state, or a new one. Requests the configured reviewer on an open review, so a retry after an
   * interrupted creation is safe. A closed review gets no request, and neither does a review its reviewer opened on
   * GitHub, which refuses a pull request's author as its reviewer.
   */
  fun findOrCreateReview(request: CreateReviewRequest): ReviewReference
  fun review(reference: ReviewReference): ReviewStatus

  /** The Git-format diff of one file between two revisions; see [UnifiedDiff.between]. */
  fun diff(before: String, after: String, beforePath: String, afterPath: String, context: Int = 200): String =
    UnifiedDiff.between(fileOrNull(before, beforePath), fileOrNull(after, afterPath), beforePath, afterPath, context)

  fun reviewWithEvidence(reference: ReviewReference): ReviewStatus
  fun mergedReview(reference: ReviewReference): MergedReview =
    error("Exact merged-change evidence is unavailable for ${reference.url}")
}

/**
 * Creates the client for [repository], with the provider token from the server environment. It leaves out what the
 * provider marks as a bot's, and what the accounts named in [bots] wrote.
 */
internal fun getReviewAPI(repository: ReviewRepository, bots: Set<String>): ReviewApi = when (repository.provider) {
  CiProviderId.GITHUB -> gitHubReviewApi(repository, bots)
  CiProviderId.SPACE -> spaceReviewApi(repository, bots)
}

private fun ReviewApi.fileOrNull(revision: String, path: String): String? = try {
  file(revision, path)
}
catch (_: NoSuchFileException) {
  null
}
