// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.support.Exchange
import org.jetbrains.qodana.edict.ci.support.ProviderRequest
import org.jetbrains.qodana.edict.ci.support.answer
import org.jetbrains.qodana.edict.ci.support.answeredWith
import org.jetbrains.qodana.edict.common.DEFAULT_EDICT_STATE_BRANCH
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.StateBranch
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.support.GitOrigin
import org.jetbrains.qodana.edict.core.EdictJson
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.feedback.support.FINDING_MESSAGE
import org.jetbrains.qodana.edict.feedback.support.HEAD_REVISION
import org.jetbrains.qodana.edict.feedback.support.OWN_PULL_REQUEST
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST_FILES
import org.jetbrains.qodana.edict.feedback.support.feedbackScripted
import org.jetbrains.qodana.edict.feedback.support.internalResult
import org.jetbrains.qodana.edict.feedback.support.sarifReport
import org.jetbrains.qodana.edict.feedback.support.stateFiles
import org.jetbrains.qodana.edict.promotion.support.ClusterFixture
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_NAME
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_SUMMARY
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset
import kotlin.io.path.exists
import kotlin.io.path.writeText
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse

class FeedbackPostingTest {
  @TempDir
  lateinit var directory: Path

  private val origin by lazy { GitOrigin(directory) }

  @Test
  fun `reserves the finding on origin, then comments on it`() {
    val run = preparedRun()

    run.post(FILES_ADDING_3, comment(RECORD) answeredWith answer(201, POSTED))

    assertEquals(mapOf(RECORD.id to RECORD), activeRecordsOnOrigin())
  }

  @Test
  fun `removes the checkout of the state once it commented, which the next run takes from origin again`() {
    val run = preparedRun()

    run.post(FILES_ADDING_3, comment(RECORD) answeredWith answer(201, POSTED))

    assertFalse(run.layout.stateDirectory.exists())
    assertEquals("", git(run.layout.root, "branch", "--list", DEFAULT_EDICT_STATE_BRANCH))
  }

  @Test
  fun `selects again after another run of the pull request reserved the finding first, and comments on nothing`() {
    val run = preparedRun()
    val other = RECORD.copy(createdAt = "2026-11-06T11:59:00Z")
    origin.raceNextStatePush("other", mapOf("feedback/active/${other.fileName}" to other.fileText()))

    run.post(FILES_ADDING_3)

    assertEquals(mapOf(other.id to other), activeRecordsOnOrigin())
  }

  @Test
  fun `reserves nothing for a finding on a line the pull request does not add`() {
    val run = preparedRun()
    val state = stateOnOrigin()

    run.post(PULL_REQUEST_FILES answeredWith answer(200, filesAdding(10)))

    assertEquals(state, stateOnOrigin())
  }

  @Test
  fun `reads nothing from GitHub when the inspections found nothing`() {
    val run = preparedRun(report = sarifReport())
    val state = stateOnOrigin()

    run.post()

    assertEquals(state, stateOnOrigin())
  }

  @Test
  fun `keeps the reservation of a comment GitHub refuses, which the routine later finds unanswered`() {
    val run = preparedRun()
    val refused = """{"message": "Resource not accessible by integration", "status": "403"}"""

    run.post(FILES_ADDING_3, comment(RECORD) answeredWith answer(403, refused))

    assertEquals(mapOf(RECORD.id to RECORD), activeRecordsOnOrigin())
  }

  @Test
  fun `fails on a report that names no full commit SHA as analysed, which a comment needs`() {
    val run = preparedRun(report = sarifReport(internalResult("rule", "src/Main.kt", startLine = 3), revision = ""))
    val state = stateOnOrigin()

    val failure = assertFailsWith<IllegalStateException> { run.post() }

    assertEquals("${run.sarif} names no full commit SHA as the analysed revision: ''", failure.message)
    assertEquals(state, stateOnOrigin())
  }

  @Test
  fun `removes the checkout of the state when the post fails too`() {
    val run = preparedRun(report = sarifReport(internalResult("rule", "src/Main.kt", startLine = 3), revision = ""))

    assertFailsWith<IllegalStateException> { run.post() }

    assertFalse(run.layout.stateDirectory.exists())
  }

  /**
   * A CI run of [OWN_PULL_REQUEST] after `prepare` selected cluster `rule`, in [layout]: its state checked out outside the
   * project, and its scan of [HEAD_REVISION] wrote [sarif].
   */
  private class Run(val layout: EdictLayout, val sarif: Path) {
    /** Runs the feedback step on a client that sends exactly [exchanges]. */
    fun post(vararg exchanges: Exchange) {
      val clock = Clock.fixed(CREATED_AT, ZoneOffset.UTC)
      feedbackScripted(*exchanges).replay { postFeedback(layout, DEFAULT_EDICT_STATE_BRANCH, OWN_PULL_REQUEST, sarif, this, clock) }
    }
  }

  /** A run whose scan wrote [report]. */
  private fun preparedRun(report: String = sarifReport(internalResult("rule", "src/Main.kt", startLine = 3))): Run {
    origin.publishState("routine", stateFiles(directory.resolve("routine-state"), ClusterFixture(emptyList())))
    val project = origin.clone("ci")
    val layout = EdictLayout(project, directory.resolve("state"))
    StateBranch(layout, DEFAULT_EDICT_STATE_BRANCH).checkOut(shallow = true)
    val sarif = directory.resolve("qodana.sarif.json").apply { writeText(report) }
    return Run(layout, sarif)
  }

  /** The commit the state branch of origin is at. */
  private fun stateOnOrigin(): String = git(origin.path, "rev-parse", DEFAULT_EDICT_STATE_BRANCH)

  /** The records awaiting an answer on the state branch of origin, by id. */
  private fun activeRecordsOnOrigin(): Map<String, FeedbackRecord> {
    val active = "$DEFAULT_EDICT_STATE_BRANCH:feedback/active"
    val files = git(origin.path, "ls-tree", "--name-only", active).lines().filter(String::isNotEmpty)
    return files.associate { file ->
      val record = EdictJson.decodeFromString(FeedbackRecord.serializer(), git(origin.path, "show", "$active/$file"))
      record.id to record
    }
  }
}

private val CREATED_AT = Instant.parse("2026-11-06T12:00:00Z")

/** The record of the finding of cluster `rule` on line 3 of `src/Main.kt` of [PULL_REQUEST] at [HEAD_REVISION]. */
private val RECORD = FeedbackRecord(
  id = "f-cbcb81a836de388928f6cb5a",
  clusterId = "rule",
  pullRequest = PULL_REQUEST,
  revision = HEAD_REVISION,
  path = "src/Main.kt",
  lines = EdictLineRange(3, 3),
  commentLine = 3,
  inspectionName = INSPECTION_NAME,
  inspectionDescription = INSPECTION_SUMMARY,
  message = FINDING_MESSAGE,
  createdAt = CREATED_AT.toString(),
)

/** The files of [PULL_REQUEST] add line 3 of `src/Main.kt`, where the finding is. */
private val FILES_ADDING_3 = PULL_REQUEST_FILES answeredWith answer(200, filesAdding(3))

private const val POSTED = """{"id": 1, "html_url": "https://github.com/owner/project/pull/7#discussion_r1"}"""

/** The files of [PULL_REQUEST]: `src/Main.kt`, where it adds only line [line]. */
private fun filesAdding(line: Int): String =
  """[{"filename": "src/Main.kt", "status": "modified", "patch": "@@ -$line,0 +$line,1 @@\n+  blockingCall()"}]"""

/** The request that comments on [record]. */
private fun comment(record: FeedbackRecord) = ProviderRequest(
  "POST",
  "/repos/owner/project/pulls/7/comments",
  body = buildJsonObject {
    put("body", feedbackComment(record))
    put("commit_id", record.revision)
    put("path", record.path)
    put("line", record.commentLine)
    put("side", "RIGHT")
  },
)

private fun git(directory: Path, vararg arguments: String): String = runProcess(directory, listOf("git", *arguments))
