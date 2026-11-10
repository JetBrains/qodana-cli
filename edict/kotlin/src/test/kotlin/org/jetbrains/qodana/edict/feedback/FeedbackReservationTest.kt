// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.core.EdictClusterStatus
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.core.EdictRepository
import org.jetbrains.qodana.edict.feedback.support.FINDING_MESSAGE
import org.jetbrains.qodana.edict.feedback.support.HEAD_REVISION
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST
import org.jetbrains.qodana.edict.feedback.support.feedbackRecord
import org.jetbrains.qodana.edict.feedback.support.storeActive
import org.jetbrains.qodana.edict.promotion.support.ClusterFixture
import org.jetbrains.qodana.edict.promotion.support.EvaluationFixture
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_NAME
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_SUMMARY
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.store
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import java.time.Instant
import kotlin.test.assertEquals

class FeedbackReservationTest {
  @TempDir
  lateinit var directory: Path

  private val repository: EdictRepository by lazy { promotionRepository(directory) }

  @Test
  fun `reserves the first finding of each selected inspection, the most precise inspection first`() {
    repository.store(cluster("a", precision = 0.8))
    repository.store(cluster("b", precision = 0.9))
    val candidates = listOf(candidate("a", "src/Main.kt", 5), candidate("a", "src/Main.kt", 3), candidate("b", "src/Other.kt", 1))

    val reserved = reserve(candidates)

    assertEquals(
      listOf(record("f-00bb87cf4fc8f7fa733e016b", "b", "src/Other.kt", 1), record("f-6a79b2bcd066f5babfb36b57", "a", "src/Main.kt", 3)),
      reserved,
    )
  }

  @Test
  fun `reserves nothing for a finding of an inspection the state no longer selects`() {
    // The routine discontinued the cluster while the scan ran.
    repository.store(cluster("rule").copy(status = EdictClusterStatus.Discontinued))

    assertEquals(emptyList(), reserve(listOf(candidate("rule", "src/Main.kt", 3))))
  }

  @Test
  fun `reserves only as many as the pull request has left`() {
    listOf("a", "b", "c", "d").forEach { repository.store(cluster(it)) }
    repository.paths.storeActive(feedbackRecord(0, clusterId = "a"), feedbackRecord(1, clusterId = "b"))

    val reserved = reserve(listOf(candidate("c", "src/Main.kt", 3), candidate("d", "src/Main.kt", 4)))

    assertEquals(listOf(record("f-54908543322a300620f46bb0", "c", "src/Main.kt", 3)), reserved)
  }

  @Test
  fun `the record of a finding over several lines keeps them and its id by the first, and comments on the candidate's line`() {
    repository.store(cluster("a"))
    val declaration = InternalFinding("a", "src/Main.kt", EdictLineRange(1, 5), FINDING_MESSAGE)

    val reserved = reserve(listOf(FeedbackCandidate(declaration, 3)))

    assertEquals(listOf(record("f-e62e59761fd7c173cd98985e", "a", "src/Main.kt", 1).copy(lines = declaration.lines, commentLine = 3)), reserved)
  }

  @Test
  fun `keeps the findings that have a line the pull request adds, to comment on the first of them`() {
    val added = finding("a", "src/Main.kt", 3)
    val endingOnAddedLine = InternalFinding("a", "src/Main.kt", EdictLineRange(5, 6), FINDING_MESSAGE)
    val aroundAddedLines = InternalFinding("a", "src/Main.kt", EdictLineRange(8, 12), FINDING_MESSAGE)
    val beforeAddedLine = InternalFinding("a", "src/Main.kt", EdictLineRange(13, 14), FINDING_MESSAGE)
    val inAnotherFile = finding("a", "src/Other.kt", 3)
    val findings = listOf(added, endingOnAddedLine, aroundAddedLines, beforeAddedLine, inAnotherFile)

    val kept = onAddedLines(findings, mapOf("src/Main.kt" to setOf(3, 6, 11, 9, 15)))

    assertEquals(listOf(FeedbackCandidate(added, 3), FeedbackCandidate(endingOnAddedLine, 6), FeedbackCandidate(aroundAddedLines, 9)), kept)
  }

  private fun reserve(candidates: List<FeedbackCandidate>): List<FeedbackRecord> =
    reserveFeedback(repository, FeedbackRecords(repository.paths), PULL_REQUEST, HEAD_REVISION, candidates, CREATED_AT)
}

private val CREATED_AT = Instant.parse("2026-11-06T12:00:00Z")

/** A generated cluster [id] without Signals, whose current evaluation has [precision]. */
private fun cluster(id: String, precision: Double = 0.9) =
  ClusterFixture(emptyList(), id, evaluation = EvaluationFixture(precision = precision))

/** A finding of the inspection of cluster [clusterId] on line [line] of [path]. */
private fun finding(clusterId: String, path: String, line: Int) =
  InternalFinding(clusterId, path, EdictLineRange(line, line), FINDING_MESSAGE)

/** The [finding] on line [line] of [path], which the pull request adds. */
private fun candidate(clusterId: String, path: String, line: Int) = FeedbackCandidate(finding(clusterId, path, line), line)

/** The record [id] of the [finding] of [clusterId] on line [line] of [path], reserved on [PULL_REQUEST] at [CREATED_AT]. */
private fun record(id: String, clusterId: String, path: String, line: Int) = FeedbackRecord(
  id = id,
  clusterId = clusterId,
  pullRequest = PULL_REQUEST,
  revision = HEAD_REVISION,
  path = path,
  lines = EdictLineRange(line, line),
  commentLine = line,
  inspectionName = INSPECTION_NAME,
  inspectionDescription = INSPECTION_SUMMARY,
  message = FINDING_MESSAGE,
  createdAt = CREATED_AT.toString(),
)
