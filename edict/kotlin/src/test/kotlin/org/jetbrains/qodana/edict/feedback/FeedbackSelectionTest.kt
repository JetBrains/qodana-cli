// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.common.FeedbackConfiguration
import org.jetbrains.qodana.edict.core.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.core.sha256Hex
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST
import org.jetbrains.qodana.edict.feedback.support.feedbackRecord
import org.jetbrains.qodana.edict.feedback.support.pullRequest
import org.jetbrains.qodana.edict.feedback.support.storeActive
import org.jetbrains.qodana.edict.feedback.support.storeHistory
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_NAME
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_SUMMARY
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.assertEquals

class FeedbackSelectionTest {
  @TempDir
  lateinit var directory: Path

  private val paths by lazy { EdictRepositoryDirectory(directory) }

  @Test
  fun `leaves out an inspection whose bytes already have a record on the pull request, answered ones included`() {
    paths.storeHistory(record(0, "commented"))
    // A record of the earlier bytes of a regenerated inspection.
    paths.storeActive(record(1, "regenerated", digest = digest("earlier")))

    val selection = select(inspection("commented"), inspection("regenerated"))

    assertEquals(FeedbackSelection(listOf(inspection("regenerated")), LIMITS.maxPerPullRequest - 2), selection)
  }

  @Test
  fun `leaves out an inspection whose bytes have as many records awaiting an answer as the limit`() {
    // On other pull requests, so that the pull request's own limit stays out of it; those of earlier bytes do not count.
    paths.storeActive(*Array(LIMITS.maxPerInspection) { record(it, "busy", pullRequest(100 + it)) })
    paths.storeActive(*Array(LIMITS.maxPerInspection) { record(20 + it, "regenerated", pullRequest(200 + it), digest("earlier")) })
    paths.storeActive(*Array(LIMITS.maxPerInspection - 1) { record(40 + it, "free", pullRequest(300 + it)) })

    val selection = select(inspection("busy"), inspection("regenerated"), inspection("free"))

    assertEquals(FeedbackSelection(listOf(inspection("free"), inspection("regenerated")), LIMITS.maxPerPullRequest), selection)
  }

  @Test
  fun `selects nothing once the pull request got as many comments as the limit, answered ones included`() {
    paths.storeActive(record(0, "a"), record(1, "b"))
    paths.storeHistory(record(2, "c"), record(3, "d"))

    assertEquals(FeedbackSelection(emptyList(), 0), select(inspection("rule")))
  }

  @Test
  fun `runs at most the inspection limit`() {
    val inspections = List(MAX_FEEDBACK_INSPECTIONS + 1) { inspection("rule-%02d".format(it)) }

    val selection = select(*inspections.toTypedArray())

    assertEquals(FeedbackSelection(inspections.take(MAX_FEEDBACK_INSPECTIONS), LIMITS.maxPerPullRequest), selection)
  }

  @Test
  fun `selects the most precise inspections first, then by id`() {
    // The least precise has the first id, so that an order by id alone fails.
    val selection = select(inspection("c"), inspection("a", precision = 0.8), inspection("b"))

    assertEquals(FeedbackSelection(listOf(inspection("b"), inspection("c"), inspection("a", 0.8)), LIMITS.maxPerPullRequest), selection)
  }

  private fun select(vararg inspections: FeedbackInspection): FeedbackSelection =
    selectFeedbackInspections(inspections.toList(), FeedbackRecords(paths), PULL_REQUEST, LIMITS)
}

/** Limits other than the defaults, so that a test fails when they are not the ones applied. */
private val LIMITS = FeedbackConfiguration(maxPerInspection = 4, maxPerPullRequest = 4)

/** The bytes of the inspection of cluster [clusterId] in the fixture state. */
private fun digest(clusterId: String): String = sha256Hex(clusterId)

/** The chosen inspection of cluster [clusterId], with [precision]. */
private fun inspection(clusterId: String, precision: Double = 0.9) =
  FeedbackInspection(clusterId, digest(clusterId), "inspections/$clusterId.inspection.kts", INSPECTION_NAME, INSPECTION_SUMMARY, precision)

/** Record [index] of a finding of the inspection of [clusterId], or of other bytes of it, on [pullRequest]. */
private fun record(index: Int, clusterId: String, pullRequest: org.jetbrains.qodana.edict.ci.ReviewReference = PULL_REQUEST, digest: String = digest(clusterId)) =
  feedbackRecord(index, clusterId, pullRequest, digest)
