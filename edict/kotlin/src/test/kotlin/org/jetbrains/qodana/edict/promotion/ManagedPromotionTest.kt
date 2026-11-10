// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.core.PromotionPrState
import org.jetbrains.qodana.edict.promotion.support.PROMOTION_REPOSITORY
import org.jetbrains.qodana.edict.promotion.support.SOURCE_FIX_ON_REVIEW
import org.jetbrains.qodana.edict.promotion.support.SOURCE_FIX_REVIEW
import org.jetbrains.qodana.edict.promotion.support.UNCOVERED_CORRECTION
import org.jetbrains.qodana.edict.promotion.support.checkedSourceFix
import org.jetbrains.qodana.edict.promotion.support.correctableCluster
import org.jetbrains.qodana.edict.promotion.support.fetchPrBatch
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.publishToInbox
import org.jetbrains.qodana.edict.promotion.support.store
import org.jetbrains.qodana.edict.promotion.support.storeActiveReview
import org.jetbrains.qodana.edict.promotion.support.weakCluster
import org.jetbrains.qodana.edict.promotion.support.weakSignalService
import org.jetbrains.qodana.edict.promotion.support.withManagement
import org.jetbrains.qodana.edict.support.FakeReviewApi
import org.jetbrains.qodana.edict.support.ReviewStep
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals

/** Promotion skills working beside the other managed tasks of a run. */
internal class ManagedPromotionTest {
  @TempDir
  lateinit var directory: Path

  private val repository by lazy { promotionRepository(directory) }
  private val reviews = FakeReviewApi(PROMOTION_REPOSITORY)

  @Test
  fun `PR analysis proceeds while a weak-signal review check waits for the provider`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)
    reviews.statuses[SOURCE_FIX_REVIEW] = ReviewStatus(ReviewState.OPEN)

    val checked = repository.withManagement(reviews) { state, management ->
      // The checker redacts with the run's state, as the server wires it, so both tasks share that state.
      val checker = weakSignalService(repository, reviews, redactor = state)
      reviews.whileWaitingAt(ReviewStep.STATUS, checker::checkReviews) { management.fetchPrBatch(state, PR_NUMBER) }
    }

    assertEquals(listOf(PR_SELECTION), reviews.selections)
    assertEquals(WeakSignalPromotionCheckReviewsResponse(listOf(checkedSourceFix(PromotionPrState.ON_REVIEW, ""))), checked)
  }

  @Test
  fun `a correction no example covers goes to the inbox and leaves the cluster alone`() {
    val cluster = repository.store(correctableCluster())

    repository.publishToInbox(UNCOVERED_CORRECTION)

    assertEquals(listOf(UNCOVERED_CORRECTION), repository.loadInboxSignals())
    assertEquals(cluster, repository.loadCluster("rule"))
  }
}

/** The PR the analysis coordinator fetches; the fake has none, so the batch is empty. */
private const val PR_NUMBER = 42

private val PR_SELECTION = ReviewSelection(maxPrs = 1, prNumbers = listOf(PR_NUMBER))
