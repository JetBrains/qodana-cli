// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.PromotionPrState
import org.jetbrains.qodana.edict.edictnext.WeakSignalPromotion
import org.jetbrains.qodana.edict.edictnext.sha256Hex
import org.jetbrains.qodana.edict.promotion.support.CANDIDATE
import org.jetbrains.qodana.edict.promotion.support.CHANGED_SOURCE_FIXES
import org.jetbrains.qodana.edict.promotion.support.CHANGED_SOURCE_FIX_OPERATION_ID
import org.jetbrains.qodana.edict.promotion.support.CHECKED_MERGED_SOURCE_FIX
import org.jetbrains.qodana.edict.promotion.support.CORRECTIONS
import org.jetbrains.qodana.edict.promotion.support.EquivalentPublication
import org.jetbrains.qodana.edict.promotion.support.FINDINGS
import org.jetbrains.qodana.edict.promotion.support.ForgedCorrection
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_DIGEST
import org.jetbrains.qodana.edict.promotion.support.InvalidPublication
import org.jetbrains.qodana.edict.promotion.support.POSITIVE_CORRECTION
import org.jetbrains.qodana.edict.promotion.support.PROMOTION_REPOSITORY
import org.jetbrains.qodana.edict.promotion.support.REVIEW_FEEDBACK
import org.jetbrains.qodana.edict.promotion.support.SECRET
import org.jetbrains.qodana.edict.promotion.support.SECRET_REDACTION
import org.jetbrains.qodana.edict.promotion.support.SOURCE_FIXES
import org.jetbrains.qodana.edict.promotion.support.SOURCE_FIX_ACCEPTED
import org.jetbrains.qodana.edict.promotion.support.SOURCE_FIX_ON_REVIEW
import org.jetbrains.qodana.edict.promotion.support.SOURCE_FIX_REQUEST
import org.jetbrains.qodana.edict.promotion.support.SOURCE_FIX_REVIEW
import org.jetbrains.qodana.edict.promotion.support.UnpreparedCluster
import org.jetbrains.qodana.edict.promotion.support.UnselectableFinding
import org.jetbrains.qodana.edict.promotion.support.VALIDATION
import org.jetbrains.qodana.edict.promotion.support.activeReviewsText
import org.jetbrains.qodana.edict.promotion.support.checkedSourceFix
import org.jetbrains.qodana.edict.promotion.support.correctableCluster
import org.jetbrains.qodana.edict.promotion.support.correctedCluster
import org.jetbrains.qodana.edict.promotion.support.finding
import org.jetbrains.qodana.edict.promotion.support.mergeSourceFix
import org.jetbrains.qodana.edict.promotion.support.projectReview
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.referencedCluster
import org.jetbrains.qodana.edict.promotion.support.replaceInspection
import org.jetbrains.qodana.edict.promotion.support.storeActiveReview
import org.jetbrains.qodana.edict.promotion.support.storeArchivedReview
import org.jetbrains.qodana.edict.promotion.support.storeUnreadableArchive
import org.jetbrains.qodana.edict.promotion.support.storeUnreadableCluster
import org.jetbrains.qodana.edict.promotion.support.store
import org.jetbrains.qodana.edict.promotion.support.unreferencedCluster
import org.jetbrains.qodana.edict.promotion.support.weakCluster
import org.jetbrains.qodana.edict.promotion.support.weakSignalService
import org.jetbrains.qodana.edict.promotion.support.withEvaluation
import org.jetbrains.qodana.edict.support.FakeReviewApi
import org.jetbrains.qodana.edict.support.ReviewStep
import org.junit.jupiter.api.io.TempDir
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.CsvSource
import org.junit.jupiter.params.provider.EnumSource
import java.nio.file.Path
import kotlin.io.path.readText
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

internal class WeakSignalPromotionServiceTest {
  @TempDir
  lateinit var directory: Path

  private val repository by lazy { promotionRepository(directory) }
  private val reviews = FakeReviewApi(PROMOTION_REPOSITORY)
  private val service by lazy { weakSignalService(repository, reviews) }

  // prepare

  @Test
  fun `a weak cluster's candidate holds its true positives by path and line`() {
    repository.store(weakCluster())

    assertEquals(WeakSignalPreparationResponse(listOf(CANDIDATE), emptyList()), service.prepare())
  }

  @Test
  fun `a candidate holds the first five distinct true positives`() {
    repository.store(weakCluster(findings = (7 downTo 1).map(::finding) + finding(1)))

    assertEquals(WeakSignalPreparationResponse(listOf(CANDIDATE.copy(findings = (1..5).map(::finding))), emptyList()), service.prepare())
  }

  @ParameterizedTest
  @EnumSource(UnselectableFinding::class)
  fun `a true positive a source fix cannot select is left out`(unselectable: UnselectableFinding) {
    repository.store(weakCluster(findings = FINDINGS + unselectable.finding))

    assertEquals(WeakSignalPreparationResponse(listOf(CANDIDATE), emptyList()), service.prepare())
  }

  @Test
  fun `a cluster whose strong signals have no references is a candidate`() {
    repository.store(unreferencedCluster().withEvaluation(projectReview()))

    assertEquals(WeakSignalPreparationResponse(listOf(CANDIDATE), emptyList()), service.prepare())
  }

  @Test
  fun `a cluster with five referenced source files is no candidate`() {
    repository.store(referencedCluster().withEvaluation(projectReview()))

    assertEquals(WeakSignalPreparationResponse(emptyList(), emptyList()), service.prepare())
  }

  @Test
  fun `a cluster that is not generated is no candidate`() {
    repository.store(weakCluster().copy(status = EdictNextClusterStatus.Pending))

    assertEquals(WeakSignalPreparationResponse(emptyList(), emptyList()), service.prepare())
  }

  @Test
  fun `a cluster without true positives is no candidate`() {
    repository.store(weakCluster(findings = emptyList()))

    assertEquals(WeakSignalPreparationResponse(emptyList(), emptyList()), service.prepare())
  }

  @Test
  fun `a cluster with an active source-fix review is no candidate`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)

    assertEquals(WeakSignalPreparationResponse(emptyList(), emptyList()), service.prepare())
  }

  @ParameterizedTest
  @EnumSource(UnpreparedCluster::class)
  fun `a cluster whose evaluation offers no candidate fails`(unprepared: UnpreparedCluster) {
    repository.store(unprepared.cluster)

    assertEquals(WeakSignalPreparationResponse(emptyList(), listOf(PromotionFailure("rule", unprepared.message))), service.prepare())
  }

  @Test
  fun `a cluster with an active review still reports its broken evaluation`() {
    repository.store(UnpreparedCluster.STALE_EVALUATION.cluster)
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)

    assertEquals(
      WeakSignalPreparationResponse(emptyList(), listOf(PromotionFailure("rule", UnpreparedCluster.STALE_EVALUATION.message))),
      service.prepare(),
    )
  }

  @Test
  fun `a failing cluster does not keep the others from being prepared`() {
    repository.store(weakCluster())
    repository.store(UnpreparedCluster.NO_PROJECT_REVIEW.cluster.copy(id = "broken-rule"))

    val result = service.prepare()

    assertEquals(
      WeakSignalPreparationResponse(listOf(CANDIDATE), listOf(PromotionFailure("broken-rule", UnpreparedCluster.NO_PROJECT_REVIEW.message))),
      result,
    )
  }

  @Test
  fun `only the selected clusters are prepared`() {
    repository.store(weakCluster())
    repository.store(weakCluster().copy(id = "other-rule"))

    assertEquals(WeakSignalPreparationResponse(listOf(CANDIDATE), emptyList()), service.prepare(listOf("rule")))
  }

  @Test
  fun `a selected cluster that does not exist fails`() {
    assertEquals(
      WeakSignalPreparationResponse(emptyList(), listOf(PromotionFailure("missing", "Cluster does not exist"))),
      service.prepare(listOf("missing")),
    )
  }

  @Test
  fun `an empty cluster selection is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> { service.prepare(emptyList()) }
    assertEquals("clusterIds must be nonempty and unique when provided", error.message)
  }

  @Test
  fun `a cluster selection with a repeated cluster is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> { service.prepare(listOf("rule", "rule")) }
    assertEquals("clusterIds must be nonempty and unique when provided", error.message)
  }

  // publish

  @Test
  fun `publishing requests the source-fix review`() {
    repository.store(weakCluster())

    service.publish("rule", INSPECTION_DIGEST, SOURCE_FIXES, VALIDATION)

    assertEquals(listOf(SOURCE_FIX_REQUEST), reviews.requests)
  }

  @Test
  fun `a published source-fix review is active on review`() {
    repository.store(weakCluster())

    val result = service.publish("rule", INSPECTION_DIGEST, SOURCE_FIXES, VALIDATION)

    assertEquals(SOURCE_FIX_ON_REVIEW, result)
    assertEquals(activeReviewsText(SOURCE_FIX_ON_REVIEW), repository.paths.activeWeakSignalPromotionsPath.readText())
  }

  @Test
  fun `publishing a cluster with an active review returns that review without a request`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)

    assertEquals(SOURCE_FIX_ON_REVIEW, service.publish("rule", INSPECTION_DIGEST, CHANGED_SOURCE_FIXES, VALIDATION))
    assertEquals(emptyList(), reviews.requests)
  }

  @ParameterizedTest
  @EnumSource(EquivalentPublication::class)
  fun `the review branch depends only on the fixed files and their contents`(publication: EquivalentPublication) {
    repository.store(weakCluster())

    service.publish("rule", INSPECTION_DIGEST, publication.fixes, publication.validation)

    assertEquals(listOf(SOURCE_FIX_REQUEST.branch), reviews.requests.map { it.branch })
  }

  @Test
  fun `changed fix contents use another review branch`() {
    repository.store(weakCluster())

    service.publish("rule", INSPECTION_DIGEST, CHANGED_SOURCE_FIXES, VALIDATION)

    assertEquals(listOf("edict/weak-$CHANGED_SOURCE_FIX_OPERATION_ID"), reviews.requests.map { it.branch })
  }

  @ParameterizedTest
  @EnumSource(InvalidPublication::class)
  fun `invalid source fixes are rejected without a request`(invalid: InvalidPublication) {
    repository.store(weakCluster())

    val error = assertFailsWith<IllegalArgumentException> { service.publish("rule", INSPECTION_DIGEST, invalid.fixes, invalid.validation) }
    assertEquals(invalid.message, error.message)
    assertEquals(emptyList(), reviews.requests)
  }

  @Test
  fun `source fixes for another inspection version are rejected`() {
    repository.store(weakCluster())

    val error = assertFailsWith<IllegalArgumentException> { service.publish("rule", sha256Hex("older"), SOURCE_FIXES, VALIDATION) }
    assertEquals("Inspection changed since weak-signal selection", error.message)
  }

  @Test
  fun `source fixes for an inspection changed after its evaluation are rejected`() {
    repository.store(weakCluster())
    repository.replaceInspection("rule", "changed inspection".toByteArray())

    val error = assertFailsWith<IllegalArgumentException> { service.publish("rule", INSPECTION_DIGEST, SOURCE_FIXES, VALIDATION) }
    assertEquals("Evaluation does not match the generated inspection", error.message)
  }

  @Test
  fun `publishing a cluster with five referenced source files is rejected`() {
    repository.store(referencedCluster().withEvaluation(projectReview()))

    val error = assertFailsWith<IllegalArgumentException> { service.publish("rule", INSPECTION_DIGEST, SOURCE_FIXES, VALIDATION) }
    assertEquals(
      "Cluster 'rule' must be Generated with referenced strong positive Signals from fewer than five distinct source files",
      error.message,
    )
  }

  @Test
  fun `publishing without a promotion repository is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> {
      weakSignalService(repository, null).publish("rule", INSPECTION_DIGEST, SOURCE_FIXES, VALIDATION)
    }
    assertEquals("Weak-signal promotion requires edict.promotion.url or edict.ci.url configuration", error.message)
  }

  @Test
  fun `publishing without a reviewer is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> {
      weakSignalService(repository, reviews, promotion = null).publish("rule", INSPECTION_DIGEST, SOURCE_FIXES, VALIDATION)
    }
    assertEquals("Weak-signal promotion requires edict.promotion.reviewer configuration", error.message)
  }

  @Test
  fun `a failed review creation stores no source-fix review`() {
    repository.store(weakCluster())
    reviews.failAt(ReviewStep.CREATION, "Provider unavailable")

    val error = assertFailsWith<IllegalStateException> { service.publish("rule", INSPECTION_DIGEST, SOURCE_FIXES, VALIDATION) }
    assertEquals("Provider unavailable", error.message)
    assertEquals(emptyMap(), repository.loadActiveWeakSignalPromotions())
  }

  // checkReviews

  @ParameterizedTest
  @CsvSource("OPEN, ON_REVIEW", "CLOSED_UNMERGED, CLOSED")
  fun `an unmerged review is reported with its state and discussion`(remote: ReviewState, state: PromotionPrState) {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)
    reviews.statuses[SOURCE_FIX_REVIEW] = ReviewStatus(remote, REVIEW_FEEDBACK)

    assertEquals(WeakSignalPromotionCheckReviewsResponse(listOf(checkedSourceFix(state, REVIEW_FEEDBACK))), service.checkReviews())
  }

  @Test
  fun `a merged review is reported with its merged change`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)
    reviews.mergeSourceFix()

    assertEquals(WeakSignalPromotionCheckReviewsResponse(listOf(CHECKED_MERGED_SOURCE_FIX)), service.checkReviews())
  }

  @Test
  fun `an open review stays active on review without its discussion`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)
    reviews.statuses[SOURCE_FIX_REVIEW] = ReviewStatus(ReviewState.OPEN, REVIEW_FEEDBACK)

    service.checkReviews()

    assertEquals(activeReviewsText(SOURCE_FIX_ON_REVIEW), repository.paths.activeWeakSignalPromotionsPath.readText())
  }

  @Test
  fun `a merged review stays active as accepted`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)
    reviews.mergeSourceFix()

    service.checkReviews()

    assertEquals(mapOf("rule" to listOf(SOURCE_FIX_ACCEPTED)), repository.loadActiveWeakSignalPromotions())
  }

  @Test
  fun `a review closed unmerged is archived`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)
    reviews.statuses[SOURCE_FIX_REVIEW] = ReviewStatus(ReviewState.CLOSED_UNMERGED)

    service.checkReviews()

    assertEquals(emptyMap(), repository.loadActiveWeakSignalPromotions())
    assertEquals(listOf(WeakSignalPromotion(SOURCE_FIX_REVIEW, PromotionPrState.CLOSED)), repository.loadWeakSignalPromotionHistory("rule"))
  }

  @Test
  fun `an archived review is not checked`() {
    repository.store(weakCluster())
    repository.storeArchivedReview(SOURCE_FIX_ACCEPTED)

    assertEquals(WeakSignalPromotionCheckReviewsResponse(), service.checkReviews())
    assertEquals(emptyList(), reviews.statusReads)
  }

  @Test
  fun `review discussion is redacted`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)
    reviews.statuses[SOURCE_FIX_REVIEW] = ReviewStatus(ReviewState.OPEN, "Do not expose $SECRET")

    val result = weakSignalService(repository, reviews, redactor = SECRET_REDACTION).checkReviews()

    assertEquals(
      WeakSignalPromotionCheckReviewsResponse(listOf(checkedSourceFix(PromotionPrState.ON_REVIEW, "Do not expose [redacted]"))),
      result,
    )
  }

  @Test
  fun `review discussion is cut to 32000 characters`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)
    reviews.statuses[SOURCE_FIX_REVIEW] = ReviewStatus(ReviewState.OPEN, "x".repeat(40_000))

    assertEquals(
      WeakSignalPromotionCheckReviewsResponse(listOf(checkedSourceFix(PromotionPrState.ON_REVIEW, "x".repeat(32_000)))),
      service.checkReviews(),
    )
  }

  @Test
  fun `a review that cannot be read is reported and stays as it was`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ACCEPTED)
    reviews.failAt(ReviewStep.STATUS, "Review unavailable")

    val result = service.checkReviews()

    assertEquals(
      WeakSignalPromotionCheckReviewsResponse(failures = listOf(PromotionReviewFailure("rule", "1", "Review unavailable"))),
      result,
    )
    assertEquals(mapOf("rule" to listOf(SOURCE_FIX_ACCEPTED)), repository.loadActiveWeakSignalPromotions())
  }

  @Test
  fun `a review read failure is redacted`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)
    reviews.failAt(ReviewStep.STATUS, "Token $SECRET was rejected")

    val result = weakSignalService(repository, reviews, redactor = SECRET_REDACTION).checkReviews()

    assertEquals(
      WeakSignalPromotionCheckReviewsResponse(failures = listOf(PromotionReviewFailure("rule", "1", "Token [redacted] was rejected"))),
      result,
    )
  }

  @Test
  fun `a merged review whose change cannot be read is accepted and reported as a failure`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)
    reviews.mergeSourceFix()
    reviews.failAt(ReviewStep.MERGE, "Merged change unavailable")

    val result = service.checkReviews()

    assertEquals(
      WeakSignalPromotionCheckReviewsResponse(
        listOf(checkedSourceFix(PromotionPrState.ACCEPTED, REVIEW_FEEDBACK)),
        listOf(PromotionReviewFailure("rule", "1", "Merged change unavailable")),
      ),
      result,
    )
    assertEquals(mapOf("rule" to listOf(SOURCE_FIX_ACCEPTED)), repository.loadActiveWeakSignalPromotions())
  }

  @Test
  fun `a review check reads no other cluster`() {
    repository.store(weakCluster())
    repository.storeUnreadableCluster("broken-rule")
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)
    reviews.mergeSourceFix()

    assertEquals(WeakSignalPromotionCheckReviewsResponse(listOf(CHECKED_MERGED_SOURCE_FIX)), service.checkReviews())
  }

  @Test
  fun `a review check reads no archived review`() {
    repository.store(weakCluster())
    repository.storeUnreadableArchive()
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)
    reviews.statuses[SOURCE_FIX_REVIEW] = ReviewStatus(ReviewState.OPEN)

    assertEquals(WeakSignalPromotionCheckReviewsResponse(listOf(checkedSourceFix(PromotionPrState.ON_REVIEW, ""))), service.checkReviews())
  }

  // completeReview

  @Test
  fun `completing an accepted review archives it`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ACCEPTED)

    service.completeReview("rule", SOURCE_FIX_REVIEW)

    assertEquals(emptyMap(), repository.loadActiveWeakSignalPromotions())
    assertEquals(listOf(SOURCE_FIX_ACCEPTED), repository.loadWeakSignalPromotionHistory("rule"))
  }

  @Test
  fun `completing an archived review again changes nothing`() {
    repository.store(weakCluster())
    repository.storeArchivedReview(SOURCE_FIX_ACCEPTED)

    service.completeReview("rule", SOURCE_FIX_REVIEW)

    assertEquals(listOf(SOURCE_FIX_ACCEPTED), repository.loadWeakSignalPromotionHistory("rule"))
  }

  @Test
  fun `completing a review still on review is rejected`() {
    repository.store(weakCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)

    val error = assertFailsWith<IllegalArgumentException> { service.completeReview("rule", SOURCE_FIX_REVIEW) }
    assertEquals("Review is still open", error.message)
  }

  @Test
  fun `completing an unknown review is rejected`() {
    repository.store(weakCluster())

    val error = assertFailsWith<IllegalArgumentException> { service.completeReview("rule", SOURCE_FIX_REVIEW) }
    assertEquals("Unknown weak-signal review", error.message)
  }

  // recordSignals

  @Test
  fun `merged corrections are recorded in the source cluster as processed signals`() {
    val cluster = repository.store(correctableCluster())
    repository.storeActiveReview(SOURCE_FIX_ACCEPTED)
    reviews.mergeSourceFix()

    val result = service.recordSignals("rule", SOURCE_FIX_REVIEW, INSPECTION_DIGEST, CORRECTIONS)

    val recorded = repository.loadCluster("rule")
    assertEquals(CORRECTIONS, result)
    assertEquals((cluster.signals + CORRECTIONS).toSet(), recorded.signals.toSet())
    assertEquals(cluster.manifest.copy(processedSignalIds = cluster.signalIds + CORRECTIONS.map(EdictNextSignal::id)), recorded.manifest)
    assertEquals(emptyList(), repository.loadInboxSignals())
  }

  @Test
  fun `recording corrections again changes nothing`() {
    val cluster = repository.store(correctedCluster())
    repository.storeActiveReview(SOURCE_FIX_ACCEPTED)
    reviews.mergeSourceFix()

    val result = service.recordSignals("rule", SOURCE_FIX_REVIEW, INSPECTION_DIGEST, CORRECTIONS)

    assertEquals(CORRECTIONS, result)
    assertEquals(cluster, repository.loadCluster("rule"))
  }

  @ParameterizedTest
  @EnumSource(ForgedCorrection::class)
  fun `a correction that misstates the merged change is rejected and changes nothing`(forged: ForgedCorrection) {
    val cluster = repository.store(correctableCluster())
    repository.storeActiveReview(SOURCE_FIX_ACCEPTED)
    reviews.mergeSourceFix()

    val error = assertFailsWith<IllegalArgumentException> {
      service.recordSignals("rule", SOURCE_FIX_REVIEW, INSPECTION_DIGEST, listOf(forged.correction))
    }
    assertEquals(forged.message, error.message)
    assertEquals(cluster, repository.loadCluster("rule"))
  }

  @Test
  fun `a correction conflicting with a recorded one is rejected and changes nothing`() {
    val cluster = repository.store(correctedCluster())
    repository.storeActiveReview(SOURCE_FIX_ACCEPTED)
    reviews.mergeSourceFix()

    val error = assertFailsWith<IllegalArgumentException> {
      service.recordSignals("rule", SOURCE_FIX_REVIEW, INSPECTION_DIGEST, listOf(POSITIVE_CORRECTION.copy(description = "Conflicting reinterpretation")))
    }
    assertEquals("Conflicting signal '${POSITIVE_CORRECTION.id}'", error.message)
    assertEquals(cluster, repository.loadCluster("rule"))
  }

  @Test
  fun `a correction without a representative example is rejected`() {
    repository.store(correctableCluster())
    repository.storeActiveReview(SOURCE_FIX_ACCEPTED)
    reviews.mergeSourceFix()

    val error = assertFailsWith<IllegalArgumentException> {
      service.recordSignals("rule", SOURCE_FIX_REVIEW, INSPECTION_DIGEST, listOf(POSITIVE_CORRECTION.copy(syntheticExampleId = null)))
    }
    assertEquals("Direct evidence must reuse an existing strong representative example; otherwise publish to inbox", error.message)
  }

  @Test
  fun `corrections of another cluster's review are rejected`() {
    repository.store(correctableCluster())
    repository.storeActiveReview(SOURCE_FIX_ACCEPTED)

    val error = assertFailsWith<IllegalArgumentException> {
      service.recordSignals("other-rule", SOURCE_FIX_REVIEW, INSPECTION_DIGEST, CORRECTIONS)
    }
    assertEquals("Review does not belong to source cluster 'other-rule'", error.message)
  }

  @Test
  fun `corrections of a review still on review are rejected`() {
    repository.store(correctableCluster())
    repository.storeActiveReview(SOURCE_FIX_ON_REVIEW)

    val error = assertFailsWith<IllegalArgumentException> {
      service.recordSignals("rule", SOURCE_FIX_REVIEW, INSPECTION_DIGEST, CORRECTIONS)
    }
    assertEquals("Review does not belong to source cluster 'rule'", error.message)
  }

  @Test
  fun `corrections analyzed for another inspection version are rejected`() {
    repository.store(correctableCluster())
    repository.storeActiveReview(SOURCE_FIX_ACCEPTED)
    reviews.mergeSourceFix()

    val error = assertFailsWith<IllegalArgumentException> {
      service.recordSignals("rule", SOURCE_FIX_REVIEW, sha256Hex("older"), CORRECTIONS)
    }
    assertEquals("Inspection changed since review analysis", error.message)
  }

  @Test
  fun `a repeated correction is rejected`() {
    repository.store(correctableCluster())
    repository.storeActiveReview(SOURCE_FIX_ACCEPTED)
    reviews.mergeSourceFix()

    val error = assertFailsWith<IllegalArgumentException> {
      service.recordSignals("rule", SOURCE_FIX_REVIEW, INSPECTION_DIGEST, listOf(POSITIVE_CORRECTION, POSITIVE_CORRECTION))
    }
    assertEquals("Supply distinct nonempty signals", error.message)
  }

  @Test
  fun `recording no corrections is rejected`() {
    repository.store(correctableCluster())
    repository.storeActiveReview(SOURCE_FIX_ACCEPTED)

    val error = assertFailsWith<IllegalArgumentException> {
      service.recordSignals("rule", SOURCE_FIX_REVIEW, INSPECTION_DIGEST, emptyList())
    }
    assertEquals("Supply actual merged corrections, not proposed fixes", error.message)
  }

  @Test
  fun `corrections cannot be recorded while the merged change cannot be read`() {
    val cluster = repository.store(correctableCluster())
    repository.storeActiveReview(SOURCE_FIX_ACCEPTED)
    reviews.mergeSourceFix()
    reviews.failAt(ReviewStep.MERGE, "Merged change unavailable")

    val error = assertFailsWith<IllegalStateException> { service.recordSignals("rule", SOURCE_FIX_REVIEW, INSPECTION_DIGEST, CORRECTIONS) }
    assertEquals("Merged change unavailable", error.message)
    assertEquals(cluster, repository.loadCluster("rule"))
  }
}
