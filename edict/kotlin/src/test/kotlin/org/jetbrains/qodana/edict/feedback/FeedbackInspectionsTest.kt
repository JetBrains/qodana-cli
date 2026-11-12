// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.core.EdictClusterStatus
import org.jetbrains.qodana.edict.core.EdictFileRevision
import org.jetbrains.qodana.edict.core.EdictJson
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.core.EdictRepository
import org.jetbrains.qodana.edict.core.EdictSignal
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import org.jetbrains.qodana.edict.core.EdictSignalSource
import org.jetbrains.qodana.edict.core.PromotionPrState
import org.jetbrains.qodana.edict.core.stableSignalId
import org.jetbrains.qodana.edict.promotion.support.ClusterFixture
import org.jetbrains.qodana.edict.promotion.support.EvaluationFixture
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_DIGEST
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_NAME
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_SUMMARY
import org.jetbrains.qodana.edict.promotion.support.inspectionPromotion
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.store
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.writeText
import kotlin.test.assertEquals

class FeedbackInspectionsTest {
  @TempDir
  lateinit var directory: Path

  private val repository: EdictRepository by lazy { promotionRepository(directory) }

  @Test
  fun `chooses the inspection of a cluster in any state that has its evaluation`() {
    repository.store(cluster("generated"))
    repository.store(cluster("invalid").copy(status = EdictClusterStatus.Invalid))
    repository.store(cluster("pending").copy(status = EdictClusterStatus.Pending))

    assertEquals(listOf(inspection("generated"), inspection("invalid"), inspection("pending")), choose())
  }

  @Test
  fun `leaves out a cluster whose generated inspection is missing`() {
    repository.store(cluster("present"))
    repository.store(cluster("missing").copy(inspection = null))

    assertEquals(listOf(inspection("present")), choose())
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

    assertEquals(listOf(inspection("regenerated")), choose())
  }

  @Test
  fun `leaves out an inspection without evaluation`() {
    repository.store(cluster("evaluated"))
    repository.store(cluster("unevaluated").copy(evaluation = null))

    assertEquals(listOf(inspection("evaluated")), choose())
  }

  @Test
  fun `leaves out an inspection whose evaluation scored other bytes`() {
    repository.store(cluster("current"))
    repository.store(cluster("outdated").copy(evaluation = EvaluationFixture(9, 1, inspectionDigest = "0".repeat(64))))

    assertEquals(listOf(inspection("current")), choose())
  }

  @Test
  fun `needs at least the target precision`() {
    repository.store(cluster("at-target", truePositives = 4))
    repository.store(cluster("below-target", truePositives = 3))

    assertEquals(listOf(inspection("at-target", TARGET_PRECISION)), choose())
  }

  @Test
  fun `leaves out an inspection whose findings got more false-positive answers than the limit that no generation took in`() {
    // Five answers in the inbox and one not yet processed by its Pending cluster.
    storeInbox(*Array(MAX_UNPROCESSED_FALSE_POSITIVES) { falsePositive(it) })
    val pending = falsePositive(MAX_UNPROCESSED_FALSE_POSITIVES)
    repository.store(
      ClusterFixture(
        listOf(pending), "busy", EdictClusterStatus.Pending, evaluation = EvaluationFixture(truePositives = 1), unprocessed = setOf(pending.id),
      ),
    )

    assertEquals(emptyList(), choose())
  }

  @Test
  fun `counts only the unprocessed false-positive answers to the inspection's own bytes`() {
    storeInbox(*Array(MAX_UNPROCESSED_FALSE_POSITIVES) { falsePositive(it) })
    // Processed by its Generated cluster, to the bytes of another inspection, and a true positive.
    repository.store(ClusterFixture(listOf(falsePositive(10)), "processed", evaluation = EvaluationFixture(truePositives = 1)))
    storeInbox(falsePositive(11, inspectionDigest = "0".repeat(64)), falsePositive(12).copy(label = EdictSignalLabel.POSITIVE))

    assertEquals(listOf(inspection("processed", 1.0)), choose())
  }

  @Test
  fun `loads what it saved, and nothing before it saved anything`() {
    val inspections = FeedbackInspections(repository.paths)
    val empty = inspections.load()
    inspections.save(listOf(inspection("rule")))

    assertEquals(listOf(emptyList(), listOf(inspection("rule"))), listOf(empty, inspections.load()))
  }

  private fun choose(): List<FeedbackInspection> = chooseFeedbackInspections(repository, TARGET_PRECISION)

  private fun storeInbox(vararg signals: EdictSignal) {
    val inbox = repository.paths.inboxDirectory.createDirectories()
    signals.forEach { inbox.resolve("${it.id}.json").writeText(EdictJson.encodeToString(EdictSignal.serializer(), it)) }
  }
}

private const val TARGET_PRECISION = 0.8

/** The chosen inspection of cluster [id], the fixture inspection, with [precision]. */
private fun inspection(id: String, precision: Double = 0.9) =
  FeedbackInspection(id, INSPECTION_DIGEST, "inspections/$id.inspection.kts", INSPECTION_NAME, INSPECTION_SUMMARY, precision)

/** A generated cluster [id] without Signals, whose current evaluation weighs [truePositives] against one false positive. */
private fun cluster(id: String, truePositives: Int = 9) =
  ClusterFixture(emptyList(), id, evaluation = EvaluationFixture(truePositives, falsePositives = 1))

/** A user's 👎 answer to a finding of the inspection bytes of [inspectionDigest]. */
private fun falsePositive(index: Int, inspectionDigest: String = INSPECTION_DIGEST) = EdictSignal(
  id = stableSignalId("edict-feedback:f-%02d".format(index)),
  idempotencyKey = "edict-feedback:f-%02d".format(index),
  fileRevision = EdictFileRevision("src/Main.kt", "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00", listOf(EdictLineRange(3, 3))),
  source = EdictSignalSource.GitHubEvaluatedFinding(
    INSPECTION_NAME, INSPECTION_SUMMARY, "Blocking call", "https://github.com/owner/project/pull/7#discussion_r$index", emptyList(),
    inspectionDigest,
  ),
  label = EdictSignalLabel.NEGATIVE,
  description = "Not blocking",
)
