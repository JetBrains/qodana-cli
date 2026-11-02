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

  /** The file's text at [revision]; throws [NoSuchFileException] when it doesn't exist there. */
  fun file(revision: String, path: String): String

  /** The branch to open reviews against: the [configured] branch name, or the repository's default branch when null. */
  fun resolveTargetBranch(configured: String?): String

  /**
   * Pushes the [request]'s branch and returns its review: the one already opened from that branch, in any state, or a
   * new one. Requests the configured reviewer either way, so a retry after an interrupted creation is safe.
   */
  fun findOrCreateReview(request: CreateReviewRequest): ReviewReference
  fun review(reference: ReviewReference): ReviewStatus

  /** The Git-format diff of one file between two revisions; see [UnifiedDiff.between]. */
  fun diff(before: String, after: String, beforePath: String, afterPath: String, context: Int = 200): String =
    UnifiedDiff.between(fileOrNull(before, beforePath), fileOrNull(after, afterPath), beforePath, afterPath, context)

  fun reviewWithEvidence(reference: ReviewReference): ReviewStatus = review(reference)
  fun mergedReview(reference: ReviewReference): MergedReview =
    error("Exact merged-change evidence is unavailable for ${reference.url}")
}

/** Creates the client for [repository], with the provider token from the server environment. */
internal fun getReviewAPI(repository: ReviewRepository): ReviewApi = when (repository.provider) {
  CiProviderId.GITHUB -> gitHubReviewApi(repository)
  CiProviderId.SPACE -> spaceReviewApi(repository)
}

private fun ReviewApi.fileOrNull(revision: String, path: String): String? = try {
  file(revision, path)
}
catch (_: NoSuchFileException) {
  null
}
