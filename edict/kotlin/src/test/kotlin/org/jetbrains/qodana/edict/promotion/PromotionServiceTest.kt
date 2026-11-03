// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.common.PromotionConfiguration
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.PromotionPrState
import org.jetbrains.qodana.edict.edictnext.sha256Hex
import org.jetbrains.qodana.edict.promotion.support.HISTORY
import org.jetbrains.qodana.edict.promotion.support.IneligibleCluster
import org.jetbrains.qodana.edict.promotion.support.NOW
import org.jetbrains.qodana.edict.promotion.support.OTHER_RULE_PROMOTION_ID
import org.jetbrains.qodana.edict.promotion.support.PROMOTED_RULE
import org.jetbrains.qodana.edict.promotion.support.PROMOTION_REPOSITORY
import org.jetbrains.qodana.edict.promotion.support.RULE_PROMOTION_ID
import org.jetbrains.qodana.edict.promotion.support.RULE_REVIEW_REQUEST
import org.jetbrains.qodana.edict.promotion.support.RULE_TRUNK_PROMOTION_ID
import org.jetbrains.qodana.edict.promotion.support.SECRET
import org.jetbrains.qodana.edict.promotion.support.SECRET_REDACTION
import org.jetbrains.qodana.edict.promotion.support.clusterWithSource
import org.jetbrains.qodana.edict.promotion.support.foreignReview
import org.jetbrains.qodana.edict.promotion.support.inspectionPromotion
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.promotionService
import org.jetbrains.qodana.edict.promotion.support.referencedCluster
import org.jetbrains.qodana.edict.promotion.support.replaceInspection
import org.jetbrains.qodana.edict.promotion.support.review
import org.jetbrains.qodana.edict.promotion.support.store
import org.jetbrains.qodana.edict.support.FakeReviewApi
import org.jetbrains.qodana.edict.support.ReviewStep
import org.junit.jupiter.api.io.TempDir
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.EnumSource
import org.junit.jupiter.params.provider.ValueSource
import java.nio.file.Path
import kotlin.io.path.exists
import kotlin.io.path.readText
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

internal class PromotionServiceTest {
  @TempDir
  lateinit var directory: Path

  private val repository by lazy { promotionRepository(directory) }
  private val reviews = FakeReviewApi(PROMOTION_REPOSITORY)
  private val service by lazy { promotionService(repository, reviews) }

  // promote

  @Test
  fun `a generated cluster with five referenced strong positive source files is promoted`() {
    repository.store(referencedCluster())

    assertEquals(PromotionPromoteResponse(listOf(PROMOTED_RULE)), service.promote())
  }

  @Test
  fun `a promotion is stored on review and keeps the cluster's processed signals`() {
    val cluster = repository.store(referencedCluster())

    service.promote()

    assertEquals(cluster.manifest.copy(promotions = listOf(PROMOTED_RULE)), repository.loadCluster("rule").manifest)
  }

  @Test
  fun `a promotion requests the review of the inspection with its description`() {
    repository.store(referencedCluster())

    service.promote()

    assertEquals(listOf(RULE_REVIEW_REQUEST), reviews.requests)
  }

  @Test
  fun `every eligible cluster is promoted in one run`() {
    repository.store(referencedCluster())
    repository.store(referencedCluster(id = "other-rule"))

    val result = service.promote()

    assertEquals(
      PromotionPromoteResponse(listOf(
        inspectionPromotion(id = OTHER_RULE_PROMOTION_ID, clusterId = "other-rule", createdAt = NOW),
        inspectionPromotion(pullRequest = review(2), createdAt = NOW),
      )),
      result,
    )
  }

  @ParameterizedTest
  @EnumSource(IneligibleCluster::class)
  fun `an ineligible cluster is skipped without a request`(ineligible: IneligibleCluster) {
    repository.store(ineligible.cluster)

    assertEquals(PromotionPromoteResponse(), service.promote())
    assertEquals(emptyList(), reviews.requests)
  }

  @Test
  fun `a cluster with fewer than five referenced source files fails without a request`() {
    val cluster = repository.store(clusterWithSource(EdictNextSignalSource.Generated()))

    val result = service.promote()

    val message = "Promotion requires strong positive Signals from at least 5 distinct source files with PR or commit diff URLs; found 4"
    assertEquals(PromotionPromoteResponse(failures = listOf(PromotionFailure("rule", message))), result)
    assertEquals(emptyList(), reviews.requests)
    assertEquals(cluster.manifest, repository.loadCluster("rule").manifest)
  }

  @Test
  fun `a cluster without a generated inspection fails`() {
    repository.store(referencedCluster().copy(inspection = null))

    assertEquals(
      PromotionPromoteResponse(failures = listOf(PromotionFailure("rule", "generated inspection is missing"))),
      service.promote(),
    )
  }

  @Test
  fun `a generated inspection that is not UTF-8 text fails`() {
    repository.store(referencedCluster())
    repository.replaceInspection("rule", byteArrayOf(0xC3.toByte(), 0x28))

    assertEquals(
      PromotionPromoteResponse(failures = listOf(PromotionFailure("rule", "generated inspection is not UTF-8 text"))),
      service.promote(),
    )
  }

  @Test
  fun `only the selected clusters are promoted`() {
    repository.store(referencedCluster())
    repository.store(referencedCluster(id = "other-rule"))

    assertEquals(PromotionPromoteResponse(listOf(PROMOTED_RULE)), service.promote(listOf("rule")))
  }

  @Test
  fun `a selected cluster that does not exist fails`() {
    assertEquals(
      PromotionPromoteResponse(failures = listOf(PromotionFailure("missing", "cluster does not exist"))),
      service.promote(listOf("missing")),
    )
  }

  @Test
  fun `an empty cluster selection is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> { service.promote(emptyList()) }
    assertEquals("clusterIds must not be empty when provided", error.message)
  }

  @Test
  fun `a cluster selection with a repeated cluster is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> { service.promote(listOf("rule", "rule")) }
    assertEquals("clusterIds must be unique", error.message)
  }

  @Test
  fun `promotion without a promotion repository is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> { promotionService(repository, null).promote() }
    assertEquals("Inspection promotion requires edict.promotion.url or edict.ci.url configuration", error.message)
  }

  @Test
  fun `promotion without a reviewer is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> { promotionService(repository, reviews, promotion = null).promote() }
    assertEquals("Inspection promotion requires edict.promotion.reviewer configuration", error.message)
  }

  @Test
  fun `a promotion without a configured target branch targets the repository's default branch`() {
    repository.store(referencedCluster())
    val trunk = FakeReviewApi(PROMOTION_REPOSITORY, defaultBranch = "trunk")

    val result = promotionService(repository, trunk, PromotionConfiguration("reviewer")).promote()

    assertEquals(
      PromotionPromoteResponse(listOf(inspectionPromotion(id = RULE_TRUNK_PROMOTION_ID, targetBranch = "trunk", createdAt = NOW))),
      result,
    )
    assertEquals(listOf(RULE_REVIEW_REQUEST.copy(operationId = RULE_TRUNK_PROMOTION_ID, targetBranch = "trunk")), trunk.requests)
  }

  @Test
  fun `an inspection already promoted to the same target is skipped, even when its review was closed`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion(PromotionPrState.CLOSED))))

    assertEquals(PromotionPromoteResponse(), service.promote())
    assertEquals(emptyList(), reviews.requests)
  }

  @Test
  fun `a changed inspection is promoted again`() {
    val older = inspectionPromotion(PromotionPrState.ACCEPTED, id = "p-${"a".repeat(24)}", inspectionDigest = sha256Hex("older"))
    repository.store(referencedCluster(promotions = listOf(older)))

    val result = service.promote()

    assertEquals(PromotionPromoteResponse(listOf(PROMOTED_RULE)), result)
    assertEquals(listOf(older, PROMOTED_RULE), repository.loadCluster("rule").manifest.promotions)
  }

  @Test
  fun `an inspection accepted in another repository is promoted in the configured one`() {
    // Accepted: startup refuses to run while a promotion of another repository is on review.
    val foreign = inspectionPromotion(id = "p-${"f".repeat(24)}", pullRequest = foreignReview(1), state = PromotionPrState.ACCEPTED)
    repository.store(referencedCluster(promotions = listOf(foreign)))

    assertEquals(PromotionPromoteResponse(listOf(PROMOTED_RULE)), service.promote())
  }

  @Test
  fun `a promotion whose review creation fails is reported and stores nothing`() {
    val cluster = repository.store(referencedCluster())
    reviews.failAt(ReviewStep.CREATION, "Provider unavailable")

    val result = service.promote()

    assertEquals(PromotionPromoteResponse(failures = listOf(PromotionFailure("rule", "Provider unavailable"))), result)
    assertEquals(cluster.manifest, repository.loadCluster("rule").manifest)
  }

  // checkReviews

  @Test
  fun `a merged review accepts its promotion`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.MERGED)

    val result = service.checkReviews()

    val accepted = inspectionPromotion(PromotionPrState.ACCEPTED).copy(updatedAt = NOW)
    assertEquals(PromotionCheckReviewsResponse(accepted = listOf(accepted)), result)
    assertEquals(listOf(accepted), repository.loadCluster("rule").manifest.promotions)
  }

  @Test
  fun `an accepted promotion is recorded in the cluster history`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.MERGED)

    service.checkReviews()

    assertEquals(
      "${HISTORY}Promotion `$RULE_PROMOTION_ID` accepted after merged PR https://github.com/JetBrains/qodana-cli/pull/1.\n",
      repository.loadCluster("rule").historyPath.readText(),
    )
  }

  @Test
  fun `an open review changes nothing`() {
    val cluster = repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.OPEN)

    assertEquals(PromotionCheckReviewsResponse(), service.checkReviews())
    assertEquals(cluster.manifest, repository.loadCluster("rule").manifest)
  }

  @Test
  fun `a review closed unmerged awaits a decision with its discussion`() {
    val cluster = repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.CLOSED_UNMERGED, "Please do not ship this rule")

    val result = service.checkReviews()

    val undecided = UndecidedPromotionReview("rule", RULE_PROMOTION_ID, review(1).url, "Please do not ship this rule")
    assertEquals(PromotionCheckReviewsResponse(undecided = listOf(undecided)), result)
    assertEquals(cluster.manifest, repository.loadCluster("rule").manifest)
  }

  @Test
  fun `closed review discussion is redacted`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.CLOSED_UNMERGED, "Do not expose $SECRET")

    val result = promotionService(repository, reviews, redactor = SECRET_REDACTION).checkReviews()

    assertEquals(
      PromotionCheckReviewsResponse(undecided = listOf(UndecidedPromotionReview("rule", RULE_PROMOTION_ID, review(1).url, "Do not expose [redacted]"))),
      result,
    )
  }

  @Test
  fun `closed review discussion is cut to 32000 characters`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.CLOSED_UNMERGED, "x".repeat(40_000))

    val result = service.checkReviews()

    assertEquals(
      PromotionCheckReviewsResponse(undecided = listOf(UndecidedPromotionReview("rule", RULE_PROMOTION_ID, review(1).url, "x".repeat(32_000)))),
      result,
    )
  }

  @Test
  fun `an accepted promotion is not checked`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion(PromotionPrState.ACCEPTED))))

    assertEquals(PromotionCheckReviewsResponse(), service.checkReviews())
    assertEquals(emptyList(), reviews.statusReads)
  }

  @Test
  fun `a closed promotion of a cluster no longer generated is not checked`() {
    val closed = referencedCluster(promotions = listOf(inspectionPromotion(PromotionPrState.CLOSED)))
    repository.store(closed.copy(status = EdictNextClusterStatus.Discontinued))

    assertEquals(PromotionCheckReviewsResponse(), service.checkReviews())
    assertEquals(emptyList(), reviews.statusReads)
  }

  @Test
  fun `a closed promotion whose review has merged since fails`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion(PromotionPrState.CLOSED))))
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.MERGED)

    val result = service.checkReviews()

    val failure = PromotionReviewFailure("rule", RULE_PROMOTION_ID, "Promotion is CLOSED, but its review is now MERGED")
    assertEquals(PromotionCheckReviewsResponse(failures = listOf(failure)), result)
  }

  @Test
  fun `a review that cannot be read is reported and keeps its promotion on review`() {
    val cluster = repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.failAt(ReviewStep.STATUS, "Provider unavailable")

    val result = service.checkReviews()

    assertEquals(
      PromotionCheckReviewsResponse(failures = listOf(PromotionReviewFailure("rule", RULE_PROMOTION_ID, "Provider unavailable"))),
      result,
    )
    assertEquals(cluster.manifest, repository.loadCluster("rule").manifest)
  }

  @Test
  fun `a review read failure is redacted`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.failAt(ReviewStep.STATUS, "Token $SECRET was rejected")

    val result = promotionService(repository, reviews, redactor = SECRET_REDACTION).checkReviews()

    assertEquals(
      PromotionCheckReviewsResponse(failures = listOf(PromotionReviewFailure("rule", RULE_PROMOTION_ID, "Token [redacted] was rejected"))),
      result,
    )
  }

  @Test
  fun `a closed promotion whose review cannot be read still awaits a decision`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion(PromotionPrState.CLOSED))))
    reviews.failAt(ReviewStep.STATUS, "Provider unavailable")

    val result = service.checkReviews()

    assertEquals(
      PromotionCheckReviewsResponse(
        undecided = listOf(UndecidedPromotionReview("rule", RULE_PROMOTION_ID, review(1).url, "")),
        failures = listOf(PromotionReviewFailure("rule", RULE_PROMOTION_ID, "Provider unavailable")),
      ),
      result,
    )
  }

  // decideReviews

  @ParameterizedTest
  @EnumSource(PromotionPrState::class, names = ["ON_REVIEW", "CLOSED"])
  fun `discontinuing a closed review discontinues the cluster`(state: PromotionPrState) {
    val cluster = repository.store(referencedCluster(promotions = listOf(inspectionPromotion(state))))
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.CLOSED_UNMERGED)

    val result = service.decideReviews(listOf(DISCONTINUE))

    val closed = inspectionPromotion(PromotionPrState.CLOSED).copy(updatedAt = NOW)
    assertEquals(PromotionDecideReviewsResponse(updated = listOf(closed)), result)
    assertEquals(
      cluster.manifest.copy(status = EdictNextClusterStatus.Discontinued, promotions = listOf(closed)),
      repository.loadCluster("rule").manifest,
    )
  }

  @Test
  fun `discontinuing a cluster removes its inspection`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.CLOSED_UNMERGED)

    service.decideReviews(listOf(DISCONTINUE))

    assertFalse(repository.paths.inspectionPath("rule").exists())
  }

  @Test
  fun `a decision is recorded in the cluster history with its rationale`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.CLOSED_UNMERGED)

    service.decideReviews(listOf(DISCONTINUE))

    assertEquals(
      "${HISTORY}Promotion `$RULE_PROMOTION_ID` moved cluster to Discontinued: Reviewer rejected the rule itself\n",
      repository.loadCluster("rule").historyPath.readText(),
    )
  }

  @Test
  fun `moving a closed review's cluster to pending keeps its inspection as the predecessor`() {
    val cluster = repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.CLOSED_UNMERGED)

    service.decideReviews(listOf(DISCONTINUE.copy(decision = ClosedReviewDecision.MOVE_CLUSTER_TO_PENDING)))

    val closed = inspectionPromotion(PromotionPrState.CLOSED).copy(updatedAt = NOW)
    assertEquals(
      cluster.manifest.copy(status = EdictNextClusterStatus.Pending, predecessorId = "rule", promotions = listOf(closed)),
      repository.loadCluster("rule").manifest,
    )
    assertTrue(repository.paths.inspectionPath("rule").exists())
  }

  @Test
  fun `a decision on a review that is still open fails and changes nothing`() {
    val cluster = repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.OPEN)

    val result = service.decideReviews(listOf(DISCONTINUE))

    val message = "Promotion '$RULE_PROMOTION_ID' remote review is OPEN, not CLOSED_UNMERGED"
    assertEquals(PromotionDecideReviewsResponse(failures = listOf(PromotionReviewFailure("rule", RULE_PROMOTION_ID, message))), result)
    assertEquals(cluster.manifest, repository.loadCluster("rule").manifest)
  }

  @Test
  fun `a decision on an accepted promotion fails`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion(PromotionPrState.ACCEPTED))))

    val result = service.decideReviews(listOf(DISCONTINUE))

    val message = "Promotion '$RULE_PROMOTION_ID' is not awaiting a closed-review decision"
    assertEquals(PromotionDecideReviewsResponse(failures = listOf(PromotionReviewFailure("rule", RULE_PROMOTION_ID, message))), result)
  }

  @Test
  fun `a decision on an unknown promotion fails`() {
    repository.store(referencedCluster())

    val result = service.decideReviews(listOf(DISCONTINUE))

    val message = "Unknown promotion '$RULE_PROMOTION_ID' in cluster 'rule'"
    assertEquals(PromotionDecideReviewsResponse(failures = listOf(PromotionReviewFailure("rule", RULE_PROMOTION_ID, message))), result)
  }

  @Test
  fun `a decision on a promotion of a replaced inspection fails and leaves the newer inspection generated`() {
    val older = inspectionPromotion(PromotionPrState.ACCEPTED, id = "p-${"4".repeat(24)}", inspectionDigest = sha256Hex("older"))
    val cluster = repository.store(referencedCluster(promotions = listOf(older, inspectionPromotion(PromotionPrState.CLOSED))))
    repository.replaceInspection("rule", "newer inspection".toByteArray())
    reviews.statuses[review(1)] = ReviewStatus(ReviewState.CLOSED_UNMERGED)

    val result = service.decideReviews(listOf(DISCONTINUE))

    val failure = PromotionReviewFailure("rule", RULE_PROMOTION_ID, "Promotion is stale: the generated inspection has changed")
    assertEquals(PromotionDecideReviewsResponse(failures = listOf(failure)), result)
    assertEquals(cluster.manifest, repository.loadCluster("rule").manifest)
  }

  @Test
  fun `a decision failure is redacted`() {
    repository.store(referencedCluster(promotions = listOf(inspectionPromotion())))
    reviews.failAt(ReviewStep.STATUS, "Token $SECRET was rejected")

    val result = promotionService(repository, reviews, redactor = SECRET_REDACTION).decideReviews(listOf(DISCONTINUE))

    assertEquals(
      PromotionDecideReviewsResponse(failures = listOf(PromotionReviewFailure("rule", RULE_PROMOTION_ID, "Token [redacted] was rejected"))),
      result,
    )
  }

  @ParameterizedTest
  @ValueSource(ints = [0, 11])
  fun `a decision batch holds one to ten decisions`(size: Int) {
    val decisions = List(size) { DISCONTINUE.copy(promotionId = "p-%024x".format(it)) }

    val error = assertFailsWith<IllegalArgumentException> { service.decideReviews(decisions) }
    assertEquals("A decision batch must contain 1 to 10 promotions", error.message)
  }

  @Test
  fun `a decision batch with two decisions of one promotion is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> { service.decideReviews(listOf(DISCONTINUE, DISCONTINUE)) }
    assertEquals("Only one decision may be supplied for each promotion", error.message)
  }

  @Test
  fun `a decision without a rationale is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> { service.decideReviews(listOf(DISCONTINUE.copy(rationale = " "))) }
    assertEquals("A rationale is required for DISCONTINUE_CLUSTER", error.message)
  }

  @Test
  fun `a decision rationale longer than 4000 characters is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> { service.decideReviews(listOf(DISCONTINUE.copy(rationale = "x".repeat(4_001)))) }
    assertEquals("Decision rationale exceeds 4000 characters", error.message)
  }
}

/** Discontinues cluster `rule` over its promotion [RULE_PROMOTION_ID]. */
private val DISCONTINUE = PromotionReviewDecision(
  "rule", RULE_PROMOTION_ID, ClosedReviewDecision.DISCONTINUE_CLUSTER, "Reviewer rejected the rule itself",
)
