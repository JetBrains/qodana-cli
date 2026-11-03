// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.support

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.MergedReview
import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewApi
import org.jetbrains.qodana.edict.ci.ReviewFetchResult
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import java.nio.file.NoSuchFileException
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Callable
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/**
 * An in-memory [repository] behind the review API. Tests put in what the code under test should find and read back
 * what it asked for. Like the real clients, it refuses reviews of other repositories and fails on reviews and files
 * it doesn't have. It is safe to call from several threads, as the MCP server does.
 */
internal class FakeReviewApi(
  override val repository: ReviewRepository = ReviewRepository(CiProviderId.GITHUB, "github.com", "owner", "repo"),
  /** The branch reviews target when none is configured. */
  private val defaultBranch: String = "main",
) : ReviewApi {
  /** The pull requests [fetch] selects from. */
  val pullRequests = CopyOnWriteArrayList<PullRequest>()

  /** The problems every [fetch] reports. */
  val problems = CopyOnWriteArrayList<String>()

  /** File texts by full revision SHA and repository-relative path. */
  val files = ConcurrentHashMap<Pair<String, String>, String>()

  /** The state and discussion of each review; a review opened here starts [ReviewState.OPEN]. */
  val statuses = ConcurrentHashMap<ReviewReference, ReviewStatus>()

  /** The changes that landed with each merged review. */
  val merges = ConcurrentHashMap<ReviewReference, MergedReview>()

  /** Runs at its step of every call, so a test can make the call fail or wait. */
  val interruptions = ConcurrentHashMap<ReviewStep, () -> Unit>()

  val selections = CopyOnWriteArrayList<ReviewSelection>()

  /** The full revision SHA and repository-relative path of each file read. */
  val fileReads = CopyOnWriteArrayList<Pair<String, String>>()

  val requests = CopyOnWriteArrayList<CreateReviewRequest>()

  /** The reviews opened here, by source branch and target branch. */
  val opened = ConcurrentHashMap<Pair<String, String>, ReviewReference>()

  /** Each review whose status was read, with or without evidence. */
  val statusReads = CopyOnWriteArrayList<ReviewReference>()

  override fun fetch(selection: ReviewSelection): ReviewFetchResult {
    selections += selection
    // Like the providers, it selects the latest merged, but the contract promises no order, so it keeps its own.
    val selected = if (selection.prNumbers.isEmpty()) {
      val latest = pullRequests.filter { selection.containsDate(it.closeTimestamp) }
        .sortedByDescending(PullRequest::closeTimestamp).take(selection.maxPrs).toSet()
      pullRequests.filter { it in latest }
    }
    else {
      selection.prNumbers.mapNotNull { number -> pullRequests.find { it.number == number } }
    }
    val missing = selection.prNumbers.filter { number -> pullRequests.none { it.number == number } }
    val reachedMaxPrs = selection.prNumbers.isEmpty() && selected.size == selection.maxPrs
    return ReviewFetchResult(selected, problems.toList() + missing.map { "No pull request $it" }, reachedMaxPrs)
  }

  override fun file(revision: String, path: String): String {
    fileReads += revision to path
    // As the contract says: a missing path at a known revision is a missing file, an unknown revision a failure.
    check(files.keys.any { it.first == revision }) { "No revision $revision" }
    return files[revision to path] ?: throw NoSuchFileException(path)
  }

  override fun resolveTargetBranch(configured: String?): String = configured ?: defaultBranch

  /** The review opened from the request's branch into its target, opening the next numbered one when there is none. */
  @Synchronized
  override fun findOrCreateReview(request: CreateReviewRequest): ReviewReference {
    requests += request
    interrupt(ReviewStep.CREATION)
    return opened.getOrPut(request.branch to request.targetBranch) {
      val number = opened.size + 1
      repository.reference("$number", "https://${repository.host}/${repository.owner}/${repository.repository}/pull/$number")
        .also { statuses.putIfAbsent(it, ReviewStatus(ReviewState.OPEN)) }
    }
  }

  /** Makes every call fail at [step] with [message]. */
  fun failAt(step: ReviewStep, message: String) {
    interruptions[step] = { error(message) }
  }

  /**
   * Runs [inFlight] until it waits at [step], runs [meanwhile] then, and only afterwards lets [inFlight] finish.
   * Returns what [inFlight] returned; fails when [meanwhile] waits for it.
   */
  fun <T> whileWaitingAt(step: ReviewStep, inFlight: () -> T, meanwhile: () -> Unit): T {
    val reached = CountDownLatch(1)
    val released = CountDownLatch(1)
    interruptions[step] = {
      reached.countDown()
      check(released.await(WAIT_SECONDS * 2, TimeUnit.SECONDS)) { "The call waiting at $step was not released" }
    }
    val executor = Executors.newFixedThreadPool(2)
    try {
      val call = executor.submit(Callable { inFlight() })
      check(reached.await(WAIT_SECONDS, TimeUnit.SECONDS)) { "No call reached $step" }
      executor.submit(Runnable { meanwhile() }).get(WAIT_SECONDS, TimeUnit.SECONDS)
      check(!call.isDone) { "The call waiting at $step finished before it was released" }
      released.countDown()
      return call.get(WAIT_SECONDS, TimeUnit.SECONDS)
    }
    finally {
      released.countDown()
      interruptions.remove(step)
      executor.shutdownNow()
    }
  }

  override fun review(reference: ReviewReference): ReviewStatus = ReviewStatus(status(reference).state)

  override fun reviewWithEvidence(reference: ReviewReference): ReviewStatus = status(reference)

  override fun mergedReview(reference: ReviewReference): MergedReview {
    repository.requireOwns(reference)
    interrupt(ReviewStep.MERGE)
    require(statuses[reference]?.state == ReviewState.MERGED) { "Review has not merged" }
    return merges[reference] ?: error("Exact merged-change evidence is unavailable for ${reference.url}")
  }

  private fun status(reference: ReviewReference): ReviewStatus {
    repository.requireOwns(reference)
    statusReads += reference
    interrupt(ReviewStep.STATUS)
    return statuses[reference] ?: error("No review ${reference.url}")
  }

  private fun interrupt(step: ReviewStep) {
    interruptions[step]?.invoke()
  }
}

/** How long [FakeReviewApi.whileWaitingAt] waits for each thread before it fails. */
private const val WAIT_SECONDS = 5L

/** A step of a [FakeReviewApi] call where a test can interrupt it. */
internal enum class ReviewStep {
  /** Before a review is found or opened. */
  CREATION,

  /** Before a review's status is read. */
  STATUS,

  /** Before a merged review's changes are read. */
  MERGE,
}
