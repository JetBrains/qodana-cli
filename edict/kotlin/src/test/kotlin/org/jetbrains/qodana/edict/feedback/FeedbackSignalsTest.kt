// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.ci.support.answeredWith
import org.jetbrains.qodana.edict.edictnext.EdictNextFileRevision
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Step
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalProvenance
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.edictnext.stableSignalId
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST_COMMENTS
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST_READ
import org.jetbrains.qodana.edict.feedback.support.commentUrl
import org.jetbrains.qodana.edict.feedback.support.edictComment
import org.jetbrains.qodana.edict.feedback.support.feedbackRecord
import org.jetbrains.qodana.edict.feedback.support.feedbackScripted
import org.jetbrains.qodana.edict.feedback.support.pullRequestIn
import org.jetbrains.qodana.edict.feedback.support.reply
import org.jetbrains.qodana.edict.feedback.support.reviewComments
import org.jetbrains.qodana.edict.feedback.support.storeActive
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_NAME
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_SUMMARY
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

// The answers are written by hand: no pull request of the fork has run the feedback yet, to record them from.
class FeedbackSignalsTest {
  @TempDir
  lateinit var directory: Path

  private val records = (1..5).map(::feedbackRecord)

  @Test
  fun `the answers are the comments with only thumbs up or only thumbs down, and the other records of an open pull request stay active`() {
    EdictNextRepositoryState.open(directory).use { store ->
      val paths = EdictRepositoryDirectory(store.root)
      paths.storeActive(*records.toTypedArray())
      // f-03 got both thumbs, f-04 none, and the comment of f-05 is gone.
      val recording = feedbackScripted(
        PULL_REQUEST_READ answeredWith pullRequestIn("open"),
        PULL_REQUEST_COMMENTS answeredWith reviewComments(
          edictComment(11, records[0], thumbsUp = 1),
          edictComment(12, records[1], thumbsDown = 2),
          edictComment(13, records[2], thumbsUp = 1, thumbsDown = 1),
          edictComment(14, records[3]),
        ),
      )

      val answered = recording.replay { FeedbackSignals(FeedbackRecords(paths), this, store).collect() }

      val expected = listOf(
        FeedbackAnswer(records[0], EdictNextSignalLabel.POSITIVE, commentUrl(11), emptyList()),
        FeedbackAnswer(records[1], EdictNextSignalLabel.NEGATIVE, commentUrl(12), emptyList()),
      )
      assertEquals(expected, answered)
      assertEquals(records, FeedbackRecords(paths).active())
    }
  }

  @Test
  fun `unanswered records of a closed pull request move to history, while answered ones wait for their Signal`() {
    EdictNextRepositoryState.open(directory).use { store ->
      val paths = EdictRepositoryDirectory(store.root)
      paths.storeActive(records[0], records[2], records[4])
      val recording = feedbackScripted(
        PULL_REQUEST_READ answeredWith pullRequestIn("closed"),
        PULL_REQUEST_COMMENTS answeredWith reviewComments(
          edictComment(11, records[0], thumbsUp = 1),
          edictComment(13, records[2], thumbsUp = 1, thumbsDown = 1),
        ),
      )

      val answered = recording.replay { FeedbackSignals(FeedbackRecords(paths), this, store).collect() }

      assertEquals(listOf(records[0]), answered.map(FeedbackAnswer::record))
      assertEquals(listOf(records[0]), FeedbackRecords(paths).active())
      assertEquals(listOf(records[2], records[4]), FeedbackRecords(paths).history())
    }
  }

  @Test
  fun `the Signal of an answer is the record's lines labelled by the thumbs, with the replies, and the record leaves the active ones`() {
    EdictNextRepositoryState.open(directory).use { store ->
      val paths = EdictRepositoryDirectory(store.root)
      paths.storeActive(records[0])
      val token = feedbackAnalysisToken(store)
      val recording = feedbackScripted(
        PULL_REQUEST_READ answeredWith pullRequestIn("open"),
        PULL_REQUEST_COMMENTS answeredWith reviewComments(
          edictComment(11, records[0], thumbsUp = 1),
          reply(12, to = 11, "alice", "It really blocks"),
        ),
      )

      val publication = recording.replay {
        FeedbackSignals(FeedbackRecords(paths), this, store).run {
          collect()
          publish(token, "f-01", "Sleeps on the worker thread")
        }
      }

      val signal = EdictNextSignal(
        id = stableSignalId("edict-feedback:f-01"),
        idempotencyKey = "edict-feedback:f-01",
        fileRevision = EdictNextFileRevision("src/Main.kt", "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00", listOf(EdictNextLineRange(3, 3))),
        source = EdictNextSignalSource.SubmittedFeedback(
          inspectionName = INSPECTION_NAME,
          inspectionDescription = INSPECTION_SUMMARY,
          problemMessage = "Blocking call on the worker thread",
          message = "alice: It really blocks",
          url = commentUrl(11),
        ),
        label = EdictNextSignalLabel.POSITIVE,
        description = "Sleeps on the worker thread",
        provenance = EdictNextSignalProvenance("f-01"),
      )
      assertEquals(EdictNextRepositoryState.SignalPublication(signal, created = true), publication)
      assertEquals(signal, store.inboxSignal(signal.id))
      assertEquals(emptyList(), FeedbackRecords(paths).active())
      assertEquals(listOf(records[0]), FeedbackRecords(paths).history())
    }
  }

  @Test
  fun `only an answer that was collected gets a Signal, and other records stay active`() {
    EdictNextRepositoryState.open(directory).use { store ->
      val paths = EdictRepositoryDirectory(store.root)
      paths.storeActive(records[2])
      val token = feedbackAnalysisToken(store)
      val recording = feedbackScripted(
        PULL_REQUEST_READ answeredWith pullRequestIn("open"),
        PULL_REQUEST_COMMENTS answeredWith reviewComments(edictComment(13, records[2], thumbsUp = 1, thumbsDown = 1)),
      )

      val failure = assertFailsWith<IllegalArgumentException> {
        recording.replay {
          FeedbackSignals(FeedbackRecords(paths), this, store).run {
            collect()
            publish(token, "f-03", "Sleeps")
          }
        }
      }

      assertEquals("No collected answer to feedback record f-03", failure.message)
      assertEquals(listOf(records[2]), FeedbackRecords(paths).active())
    }
  }

  private fun feedbackAnalysisToken(store: EdictNextRepositoryState): String {
    val plan = store.createPlan("Turn feedback answers into Signals", listOf(Step(FEEDBACK_SIGNAL_SKILL, "Answers")))
    return store.launch(plan.token, plan.plan.tasks.single().id, FEEDBACK_SIGNAL_SKILL).token
  }
}
