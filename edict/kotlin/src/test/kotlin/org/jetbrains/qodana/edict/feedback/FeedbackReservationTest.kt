// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.common.FeedbackConfiguration
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.core.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.core.sha256Hex
import org.jetbrains.qodana.edict.feedback.support.FINDING_MESSAGE
import org.jetbrains.qodana.edict.feedback.support.HEAD_REVISION
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST
import org.jetbrains.qodana.edict.feedback.support.feedbackRecord
import org.jetbrains.qodana.edict.feedback.support.storeActive
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_NAME
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_SUMMARY
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import java.time.Instant
import kotlin.test.assertEquals

class FeedbackReservationTest {
  @TempDir
  lateinit var directory: Path

  private val paths by lazy { EdictRepositoryDirectory(directory) }

  @Test
  fun `reserves the first finding of each selected inspection, in the order the state chose them`() {
    val candidates = listOf(candidate("a", "src/Main.kt", 5), candidate("a", "src/Main.kt", 3), candidate("b", "src/Other.kt", 1))

    val reserved = reserve(candidates, inspection("b"), inspection("a"))

    assertEquals(
      listOf(record("f-0a943aaf5e7fc5dd20e034d6", "b", "src/Other.kt", 1), record("f-c117ecc332fe37782a9c4f88", "a", "src/Main.kt", 3)),
      reserved,
    )
  }

  @Test
  fun `reserves nothing for a finding of an inspection the state no longer chooses`() {
    // The routine pushed a state without the inspection while the scan ran.
    assertEquals(emptyList(), reserve(listOf(candidate("rule", "src/Main.kt", 3)), inspection("other")))
  }

  @Test
  fun `reserves only as many as the pull request has left`() {
    paths.storeActive(feedbackRecord(0, clusterId = "a"), feedbackRecord(1, clusterId = "b"))

    val reserved = reserve(listOf(candidate("c", "src/Main.kt", 3), candidate("d", "src/Main.kt", 4)), inspection("c"), inspection("d"))

    assertEquals(listOf(record("f-be2f2db90c91d98d99c208cd", "c", "src/Main.kt", 3)), reserved)
  }

  @Test
  fun `the record of a finding over several lines keeps them and its id by the first, and comments on the candidate's line`() {
    val declaration = InternalFinding("a", "src/Main.kt", EdictLineRange(1, 5), FINDING_MESSAGE)

    val reserved = reserve(listOf(FeedbackCandidate(declaration, 3)), inspection("a"))

    assertEquals(listOf(record("f-b4c57a298d42337b4d99c439", "a", "src/Main.kt", 1).copy(lines = declaration.lines, commentLine = 3)), reserved)
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

  private fun reserve(candidates: List<FeedbackCandidate>, vararg inspections: FeedbackInspection): List<FeedbackRecord> =
    reserveFeedback(inspections.toList(), FeedbackRecords(paths), PULL_REQUEST, HEAD_REVISION, candidates, CREATED_AT, FeedbackConfiguration())
}

private val CREATED_AT = Instant.parse("2026-11-06T12:00:00Z")

/** The chosen inspection of cluster [clusterId], whose bytes have the digest of the id. */
private fun inspection(clusterId: String) =
  FeedbackInspection(clusterId, sha256Hex(clusterId), "inspections/$clusterId.inspection.kts", INSPECTION_NAME, INSPECTION_SUMMARY, 0.9)

/** A finding of the inspection of cluster [clusterId] on line [line] of [path]. */
private fun finding(clusterId: String, path: String, line: Int) =
  InternalFinding(clusterId, path, EdictLineRange(line, line), FINDING_MESSAGE)

/** The [finding] on line [line] of [path], which the pull request adds. */
private fun candidate(clusterId: String, path: String, line: Int) = FeedbackCandidate(finding(clusterId, path, line), line)

/** The record [id] of the [finding] of [clusterId] on line [line] of [path], reserved on [PULL_REQUEST] at [CREATED_AT]. */
private fun record(id: String, clusterId: String, path: String, line: Int) = FeedbackRecord(
  id = id,
  clusterId = clusterId,
  inspectionDigest = sha256Hex(clusterId),
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
