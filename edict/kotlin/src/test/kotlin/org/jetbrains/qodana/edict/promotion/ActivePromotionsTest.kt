// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.edictnext.PromotionPrState
import org.jetbrains.qodana.edict.edictnext.WeakSignalPromotion
import org.jetbrains.qodana.edict.promotion.support.PROMOTION_REPOSITORY
import org.jetbrains.qodana.edict.promotion.support.SOURCE_FIX_ON_REVIEW
import org.jetbrains.qodana.edict.promotion.support.foreignReview
import org.jetbrains.qodana.edict.promotion.support.inspectionPromotion
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.referencedCluster
import org.jetbrains.qodana.edict.promotion.support.review
import org.jetbrains.qodana.edict.promotion.support.store
import org.jetbrains.qodana.edict.promotion.support.storeActiveReview
import org.jetbrains.qodana.edict.support.FakeReviewApi
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

internal class ActivePromotionsTest {
  @TempDir
  lateinit var directory: Path

  private val repository by lazy { promotionRepository(directory) }
  private val reviews = FakeReviewApi(PROMOTION_REPOSITORY)

  @Test
  fun `an empty state needs no promotion repository`() {
    requireActivePromotionsOwnedBy(repository, null)
  }

  @Test
  fun `active promotions of the configured repository pass`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    repository.storeActiveReview(WeakSignalPromotion(review(2)))

    requireActivePromotionsOwnedBy(repository, reviews)
  }

  @Test
  fun `an accepted inspection promotion may belong to an earlier repository`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion(PromotionPrState.ACCEPTED, pullRequest = foreignReview(1)))))

    requireActivePromotionsOwnedBy(repository, reviews)
  }

  @Test
  fun `an inspection promotion on review in another repository fails`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion(pullRequest = foreignReview(1)))))

    val error = assertFailsWith<IllegalArgumentException> { requireActivePromotionsOwnedBy(repository, reviews) }
    assertEquals(
      "Active promotions belong to another repository than the configured github:github.com/JetBrains/qodana-cli. Restore that " +
        "configuration until they are resolved: cluster 'rule' github:github.com/JetBrains/kotlin https://github.com/JetBrains/kotlin/pull/1",
      error.message,
    )
  }

  @Test
  fun `an active weak-signal review in another repository fails`() {
    repository.storeActiveReview(WeakSignalPromotion(foreignReview(2)))

    val error = assertFailsWith<IllegalArgumentException> { requireActivePromotionsOwnedBy(repository, reviews) }
    assertEquals(
      "Active promotions belong to another repository than the configured github:github.com/JetBrains/qodana-cli. Restore that " +
        "configuration until they are resolved: cluster 'rule' github:github.com/JetBrains/kotlin https://github.com/JetBrains/kotlin/pull/2",
      error.message,
    )
  }

  @Test
  fun `every foreign active promotion is named`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion(pullRequest = foreignReview(1)))))
    repository.storeActiveReview(WeakSignalPromotion(foreignReview(2)))

    val error = assertFailsWith<IllegalArgumentException> { requireActivePromotionsOwnedBy(repository, reviews) }
    assertEquals(
      "Active promotions belong to another repository than the configured github:github.com/JetBrains/qodana-cli. Restore that " +
        "configuration until they are resolved: cluster 'rule' github:github.com/JetBrains/kotlin https://github.com/JetBrains/kotlin/pull/1; " +
        "cluster 'rule' github:github.com/JetBrains/kotlin https://github.com/JetBrains/kotlin/pull/2",
      error.message,
    )
  }

  @Test
  fun `active promotions without a promotion repository fail`() {
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)

    val error = assertFailsWith<IllegalArgumentException> { requireActivePromotionsOwnedBy(repository, null) }
    assertEquals(
      "Active promotions belong to another repository than the configured no promotion repository (edict.promotion.url or " +
        "edict.ci.url). Restore that configuration until they are resolved: cluster 'rule' github:github.com/JetBrains/qodana-cli " +
        "https://github.com/JetBrains/qodana-cli/pull/1",
      error.message,
    )
  }
}
