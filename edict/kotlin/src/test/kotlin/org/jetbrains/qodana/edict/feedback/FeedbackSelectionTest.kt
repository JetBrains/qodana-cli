// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EvaluatedInspection
import org.jetbrains.qodana.edict.edictnext.PromotionPrState
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST
import org.jetbrains.qodana.edict.feedback.support.feedbackRecord
import org.jetbrains.qodana.edict.feedback.support.pullRequest
import org.jetbrains.qodana.edict.feedback.support.storeActive
import org.jetbrains.qodana.edict.feedback.support.storeHistory
import org.jetbrains.qodana.edict.promotion.support.ClusterFixture
import org.jetbrains.qodana.edict.promotion.support.EvaluationFixture
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_DIGEST
import org.jetbrains.qodana.edict.promotion.support.inspectionPromotion
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.store
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.assertEquals

class FeedbackSelectionTest {
  @TempDir
  lateinit var directory: Path

  private val repository: EdictRepository by lazy { promotionRepository(directory) }

  @Test
  fun `ranks generated inspections by the precision of their current evaluation, then by id`() {
    // The least precise has the first id, so that an order by id alone fails.
    repository.store(cluster("a", precision = 0.8))
    repository.store(cluster("c", precision = 0.9))
    repository.store(cluster("b", precision = 0.9))

    assertEquals(listOf(inspection("b"), inspection("c"), inspection("a")), select())
  }

  @Test
  fun `leaves out a cluster whose inspection is not generated`() {
    repository.store(cluster("generated"))
    repository.store(cluster("pending").copy(status = EdictNextClusterStatus.Pending))

    assertEquals(listOf(inspection("generated")), select())
  }

  @Test
  fun `leaves out a cluster whose generated inspection is missing`() {
    repository.store(cluster("present"))
    repository.store(cluster("missing").copy(inspection = null))

    assertEquals(listOf(inspection("present")), select())
  }

  @Test
  fun `leaves out an inspection whose exact bytes a promotion proposed, but not one regenerated since`() {
    listOf(PromotionPrState.ON_REVIEW, PromotionPrState.ACCEPTED, PromotionPrState.CLOSED).forEach { state ->
      val id = state.name.lowercase().replace('_', '-')
      repository.store(cluster(id).copy(promotions = listOf(inspectionPromotion(state, clusterId = id))))
    }
    // Deciding a closed promotion moved the cluster to Pending, which generated other bytes.
    val earlierBytes = inspectionPromotion(PromotionPrState.CLOSED, clusterId = "regenerated", inspectionDigest = "0".repeat(64))
    repository.store(cluster("regenerated").copy(promotions = listOf(earlierBytes)))

    assertEquals(listOf(inspection("regenerated")), select())
  }

  @Test
  fun `leaves out an inspection without evaluation`() {
    repository.store(cluster("evaluated"))
    repository.store(cluster("unevaluated").copy(evaluation = null))

    assertEquals(listOf(inspection("evaluated")), select())
  }

  @Test
  fun `leaves out an inspection whose evaluation scored other bytes`() {
    repository.store(cluster("current"))
    repository.store(cluster("outdated").copy(evaluation = EvaluationFixture(precision = 0.9, inspectionDigest = "0".repeat(64))))

    assertEquals(listOf(inspection("current")), select())
  }

  @Test
  fun `needs a precision above the threshold`() {
    repository.store(cluster("at-threshold", precision = MIN_FEEDBACK_PRECISION))
    repository.store(cluster("above-threshold", precision = 0.71))

    assertEquals(listOf(inspection("above-threshold")), select())
  }

  @Test
  fun `leaves out an inspection whose active records reached the limit`() {
    repository.store(cluster("busy"))
    repository.store(cluster("free"))
    // On other pull requests, so that the pull request's own limit stays out of it.
    repository.paths.storeActive(*Array(MAX_ACTIVE_FEEDBACK_PER_INSPECTION) { feedbackRecord(it, "busy", pullRequest(100 + it)) })
    repository.paths.storeActive(*Array(MAX_ACTIVE_FEEDBACK_PER_INSPECTION - 1) { feedbackRecord(20 + it, "free", pullRequest(200 + it)) })

    assertEquals(listOf(inspection("free")), select())
  }

  @Test
  fun `selects nothing once the pull request got its comments, answered ones included`() {
    repository.store(cluster("rule"))
    repository.paths.storeActive(feedbackRecord(0), feedbackRecord(1))
    repository.paths.storeHistory(feedbackRecord(2))

    assertEquals(emptyList(), select())
  }

  @Test
  fun `selects for a pull request below its comment limit`() {
    repository.store(cluster("rule"))
    repository.paths.storeActive(feedbackRecord(0))
    repository.paths.storeHistory(feedbackRecord(1))

    assertEquals(listOf(inspection("rule")), select())
  }

  @Test
  fun `runs at most the inspection limit`() {
    val ids = List(MAX_FEEDBACK_INSPECTIONS + 1) { "rule-%02d".format(it) }
    ids.forEach { repository.store(cluster(it)) }

    assertEquals(ids.take(MAX_FEEDBACK_INSPECTIONS).map { inspection(it) }, select())
  }

  private fun select(): List<EvaluatedInspection> = selectFeedbackInspections(repository, FeedbackRecords(repository.paths), PULL_REQUEST)

  /** The stored inspection of cluster [id] with its stored evaluation, as a selection returns it. */
  private fun inspection(id: String): EvaluatedInspection {
    val evaluation = checkNotNull(repository.loadEvaluation(repository.loadCluster(id)))
    return EvaluatedInspection(repository.paths.inspectionPath(id), INSPECTION_DIGEST, evaluation, id)
  }
}

/** A generated cluster [id] without Signals, whose current evaluation has [precision]. */
private fun cluster(id: String, precision: Double = 0.9) =
  ClusterFixture(emptyList(), id, evaluation = EvaluationFixture(precision = precision))
