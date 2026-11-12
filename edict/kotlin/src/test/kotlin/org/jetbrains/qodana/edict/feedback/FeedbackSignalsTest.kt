// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.ci.support.answer
import org.jetbrains.qodana.edict.ci.support.answeredWith
import org.jetbrains.qodana.edict.core.EdictFileRevision
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.core.EdictReply
import org.jetbrains.qodana.edict.core.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.core.EdictRepositoryState
import org.jetbrains.qodana.edict.core.EdictRepositoryState.Step
import org.jetbrains.qodana.edict.core.EdictSignal
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import org.jetbrains.qodana.edict.core.EdictSignalProvenance
import org.jetbrains.qodana.edict.core.EdictSignalSource
import org.jetbrains.qodana.edict.core.stableSignalId
import org.jetbrains.qodana.edict.feedback.support.COLLECTED_AT
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST_COMMENTS
import org.jetbrains.qodana.edict.feedback.support.commentUrl
import org.jetbrains.qodana.edict.feedback.support.commentsOf
import org.jetbrains.qodana.edict.feedback.support.edictComment
import org.jetbrains.qodana.edict.feedback.support.feedbackRecord
import org.jetbrains.qodana.edict.feedback.support.feedbackScripted
import org.jetbrains.qodana.edict.feedback.support.listAnswer
import org.jetbrains.qodana.edict.feedback.support.listedPullRequest
import org.jetbrains.qodana.edict.feedback.support.pullRequest
import org.jetbrains.qodana.edict.feedback.support.recentPullRequests
import org.jetbrains.qodana.edict.feedback.support.reply
import org.jetbrains.qodana.edict.feedback.support.storeActive
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_DIGEST
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_NAME
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_SUMMARY
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import java.time.Clock
import java.time.ZoneOffset
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

// The answers are written by hand: no pull request of the fork has run the feedback yet, to record them from.
class FeedbackSignalsTest {
  @TempDir
  lateinit var directory: Path

  private val records = (1..5).map(::feedbackRecord)

  @Test
  fun `the answers are the comments with only thumbs up or only thumbs down, and the other records of an open pull request stay active`() {
    EdictRepositoryState.open(directory).use { store ->
      val paths = EdictRepositoryDirectory(store.root)
      paths.storeActive(*records.toTypedArray())
      // f-03 got both thumbs, f-04 none, and the comment of f-05 is gone.
      val recording = feedbackScripted(
        recentPullRequests() answeredWith listAnswer(listedPullRequest(7, "open")),
        PULL_REQUEST_COMMENTS answeredWith listAnswer(
          edictComment(11, records[0], thumbsUp = 1),
          edictComment(12, records[1], thumbsDown = 2),
          edictComment(13, records[2], thumbsUp = 1, thumbsDown = 1),
          edictComment(14, records[3]),
        ),
      )

      val collection = recording.replay { feedbackSignals(paths, this, store).collect() }

      val answers = listOf(
        FeedbackAnswer(records[0], EdictSignalLabel.POSITIVE, commentUrl(11), emptyList()),
        FeedbackAnswer(records[1], EdictSignalLabel.NEGATIVE, commentUrl(12), emptyList()),
      )
      assertEquals(FeedbackCollection(answers, emptyList()), collection)
      assertEquals(records, FeedbackRecords(paths).active())
    }
  }

  @Test
  fun `unanswered records of a closed pull request move to history, while answered ones wait for their Signal`() {
    EdictRepositoryState.open(directory).use { store ->
      val paths = EdictRepositoryDirectory(store.root)
      paths.storeActive(records[0], records[2], records[4])
      val recording = feedbackScripted(
        recentPullRequests() answeredWith listAnswer(listedPullRequest(7, "closed")),
        PULL_REQUEST_COMMENTS answeredWith listAnswer(
          edictComment(11, records[0], thumbsUp = 1),
          edictComment(13, records[2], thumbsUp = 1, thumbsDown = 1),
        ),
      )

      val collection = recording.replay { feedbackSignals(paths, this, store).collect() }

      val answer = FeedbackAnswer(records[0], EdictSignalLabel.POSITIVE, commentUrl(11), emptyList())
      assertEquals(FeedbackCollection(listOf(answer), emptyList()), collection)
      assertEquals(listOf(records[0]), FeedbackRecords(paths).active())
      assertEquals(listOf(records[2], records[4]), FeedbackRecords(paths).history())
    }
  }

  @Test
  fun `the records of a pull request not updated for a week move to history unread, freeing their inspection's slots`() {
    EdictRepositoryState.open(directory).use { store ->
      val paths = EdictRepositoryDirectory(store.root)
      paths.storeActive(records[0], records[1])
      // Pull request 7 was last updated eight days before the collection; GitHub lists the most recently updated first.
      val recording = feedbackScripted(
        recentPullRequests() answeredWith listAnswer(
          listedPullRequest(9, "open", updatedAt = "2026-11-09T08:00:00Z"),
          listedPullRequest(7, "open", updatedAt = "2026-11-02T12:00:00Z"),
        ),
      )

      val collection = recording.replay { feedbackSignals(paths, this, store).collect() }

      assertEquals(FeedbackCollection(emptyList(), emptyList()), collection)
      assertEquals(emptyList(), FeedbackRecords(paths).active())
      assertEquals(listOf(records[0], records[1]), FeedbackRecords(paths).history())
    }
  }

  @Test
  fun `a pull request GitHub cannot read keeps its records active and leaves the answers of the others`() {
    EdictRepositoryState.open(directory).use { store ->
      val paths = EdictRepositoryDirectory(store.root)
      val other = feedbackRecord(6, pullRequest = pullRequest(8))
      paths.storeActive(records[0], other)
      val recording = feedbackScripted(
        recentPullRequests() answeredWith listAnswer(listedPullRequest(8, "open"), listedPullRequest(7, "closed")),
        PULL_REQUEST_COMMENTS answeredWith answer(502, """{"message": "Server Error"}"""),
        commentsOf(pullRequest(8)) answeredWith listAnswer(edictComment(21, other, thumbsUp = 1)),
      )

      val collection = recording.replay { feedbackSignals(paths, this, store).collect() }

      val unread = UnreadPullRequest(
        PULL_REQUEST.url,
        "GitHub read of /repos/owner/project/pulls/7/comments returned HTTP 502: Server Error",
      )
      // Pull request 7 is closed, yet its record stays active: its answer may still be read.
      val answer = FeedbackAnswer(other, EdictSignalLabel.POSITIVE, commentUrl(21, pullRequest(8)), emptyList())
      assertEquals(FeedbackCollection(listOf(answer), listOf(unread)), collection)
      assertEquals(listOf(records[0], other), FeedbackRecords(paths).active())
    }
  }

  @Test
  fun `a repository whose pull requests GitHub cannot list keeps every record active`() {
    EdictRepositoryState.open(directory).use { store ->
      val paths = EdictRepositoryDirectory(store.root)
      paths.storeActive(records[0])
      val recording = feedbackScripted(recentPullRequests() answeredWith answer(404, """{"message": "Not Found"}"""))

      val collection = recording.replay { feedbackSignals(paths, this, store).collect() }

      val unread = UnreadPullRequest(
        PULL_REQUEST.url,
        "GitHub read of /repos/owner/project/pulls returned HTTP 404: Not Found",
      )
      assertEquals(FeedbackCollection(emptyList(), listOf(unread)), collection)
      assertEquals(listOf(records[0]), FeedbackRecords(paths).active())
    }
  }

  @Test
  fun `the Signal of an answer is the record's lines labelled by the thumbs, with the replies, and the record leaves the active ones`() {
    EdictRepositoryState.open(directory).use { store ->
      val paths = EdictRepositoryDirectory(store.root)
      paths.storeActive(records[0])
      val token = feedbackAnalysisToken(store)
      val recording = feedbackScripted(
        recentPullRequests() answeredWith listAnswer(listedPullRequest(7, "open")),
        PULL_REQUEST_COMMENTS answeredWith listAnswer(
          edictComment(11, records[0], thumbsUp = 1),
          reply(12, to = 11, "alice", "It really blocks"),
        ),
      )

      val publication = recording.replay {
        feedbackSignals(paths, this, store).run {
          collect()
          publish(token, "f-01", "Sleeps on the worker thread")
        }
      }

      val signal = EdictSignal(
        id = stableSignalId("edict-feedback:f-01"),
        idempotencyKey = "edict-feedback:f-01",
        fileRevision = EdictFileRevision("src/Main.kt", "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00", listOf(EdictLineRange(3, 3))),
        source = EdictSignalSource.GitHubEvaluatedFinding(
          inspectionName = INSPECTION_NAME,
          inspectionDescription = INSPECTION_SUMMARY,
          problemMessage = "Blocking call on the worker thread",
          url = commentUrl(11),
          replies = listOf(EdictReply("alice", "It really blocks")),
          inspectionDigest = INSPECTION_DIGEST,
        ),
        label = EdictSignalLabel.POSITIVE,
        description = "Sleeps on the worker thread",
        provenance = EdictSignalProvenance("f-01"),
      )
      assertEquals(EdictRepositoryState.SignalPublication(signal, created = true), publication)
      assertEquals(signal, store.inboxSignal(signal.id))
      assertEquals(emptyList(), FeedbackRecords(paths).active())
      assertEquals(listOf(records[0]), FeedbackRecords(paths).history())
    }
  }

  @Test
  fun `only an answer that was collected gets a Signal, and other records stay active`() {
    EdictRepositoryState.open(directory).use { store ->
      val paths = EdictRepositoryDirectory(store.root)
      paths.storeActive(records[2])
      val token = feedbackAnalysisToken(store)
      val recording = feedbackScripted(
        recentPullRequests() answeredWith listAnswer(listedPullRequest(7, "open")),
        PULL_REQUEST_COMMENTS answeredWith listAnswer(edictComment(13, records[2], thumbsUp = 1, thumbsDown = 1)),
      )

      val failure = assertFailsWith<IllegalArgumentException> {
        recording.replay {
          feedbackSignals(paths, this, store).run {
            collect()
            publish(token, "f-03", "Sleeps")
          }
        }
      }

      assertEquals("No collected answer to feedback record f-03", failure.message)
      assertEquals(listOf(records[2]), FeedbackRecords(paths).active())
    }
  }

  private fun feedbackSignals(paths: EdictRepositoryDirectory, api: GitHubFeedbackApi, store: EdictRepositoryState) =
    FeedbackSignals(FeedbackRecords(paths), api, store, Clock.fixed(COLLECTED_AT, ZoneOffset.UTC))

  private fun feedbackAnalysisToken(store: EdictRepositoryState): String {
    val plan = store.createPlan("Turn feedback answers into Signals", listOf(Step(FEEDBACK_SIGNAL_SKILL, "Answers")))
    return store.launch(plan.token, plan.plan.tasks.single().id, FEEDBACK_SIGNAL_SKILL).token
  }
}
