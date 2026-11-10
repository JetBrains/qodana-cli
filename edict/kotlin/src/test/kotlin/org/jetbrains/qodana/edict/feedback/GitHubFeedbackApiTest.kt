// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.support.ProviderRequest
import org.jetbrains.qodana.edict.ci.support.answer
import org.jetbrains.qodana.edict.ci.support.answeredWith
import org.jetbrains.qodana.edict.feedback.support.HEAD_REVISION
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST_COMMENTS
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST_FILES
import org.jetbrains.qodana.edict.feedback.support.commentUrl
import org.jetbrains.qodana.edict.feedback.support.edictComment
import org.jetbrains.qodana.edict.feedback.support.COLLECTED_AT
import org.jetbrains.qodana.edict.feedback.support.PROJECT
import org.jetbrains.qodana.edict.feedback.support.listAnswer
import org.jetbrains.qodana.edict.feedback.support.listedPullRequest
import org.jetbrains.qodana.edict.feedback.support.recentPullRequests
import org.jetbrains.qodana.edict.feedback.support.reply
import org.jetbrains.qodana.edict.feedback.support.feedbackRecord
import org.jetbrains.qodana.edict.feedback.support.feedbackScripted
import org.jetbrains.qodana.edict.core.EdictReply
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import org.junit.jupiter.api.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

// The answers are written by hand: no pull request of the fork has run the feedback yet, to record them from.
class GitHubFeedbackApiTest {
  @Test
  fun `a pull request adds the lines that the patches of its files add`() {
    val files = """
      [
        {"filename": "src/Main.kt", "status": "modified",
         "patch": "@@ -2,3 +2,4 @@ fun main() {\n   val a = 1\n-  val b = 2\n+  val b = 3\n+  val c = 4\n   println(a)"},
        {"filename": "src/New.kt", "status": "added", "patch": "@@ -0,0 +1,2 @@\n+class New\n+fun create() = New()"},
        {"filename": "logo.png", "status": "added"}
      ]
    """.trimIndent()
    val recording = feedbackScripted(PULL_REQUEST_FILES answeredWith answer(200, files))

    val added = recording.replay { addedLines(PULL_REQUEST) }

    // GitHub shows no patch of a binary file.
    assertEquals(mapOf("src/Main.kt" to setOf(3, 4), "src/New.kt" to setOf(1, 2)), added)
  }

  @Test
  fun `a pull request answers the comments of Edict with their thumbs and replies, apart from other review comments`() {
    val recording = feedbackScripted(
      PULL_REQUEST_COMMENTS answeredWith listAnswer(
        edictComment(11, feedbackRecord(1), thumbsUp = 2),
        """{"id": 12, "body": "Rename this", "html_url": "${commentUrl(12)}", "user": {"login": "bob"}, "reactions": {"+1": 1, "-1": 0}}""",
        reply(13, to = 11, "alice", "It really blocks"),
        edictComment(14, feedbackRecord(2)),
        reply(15, to = 12, "carol", "Agreed"),
      ),
    )

    val read = recording.replay { threads(PULL_REQUEST) }.getOrThrow()

    val expectedThreads = mapOf(
      "f-01" to FeedbackThread(commentUrl(11), EdictSignalLabel.POSITIVE, listOf(EdictReply("alice", "It really blocks"))),
      "f-02" to FeedbackThread(commentUrl(14), null, emptyList()),
    )
    assertEquals(expectedThreads, read)
  }

  @Test
  fun `the recent pull requests are those updated since the given time, open or closed`() {
    val recording = feedbackScripted(
      recentPullRequests() answeredWith listAnswer(
        listedPullRequest(9, "open", updatedAt = "2026-11-09T08:00:00Z"),
        listedPullRequest(8, "closed", updatedAt = "2026-11-08T08:00:00Z"),
        listedPullRequest(7, "open", updatedAt = "2026-11-01T08:00:00Z"),
      ),
    )

    val recent = recording.replay { recentPullRequests(PROJECT, SINCE, limit = 200) }.getOrThrow()

    assertEquals(mapOf("9" to true, "8" to false), recent)
  }

  @Test
  fun `the recent pull requests are at most the limit, the most recently updated`() {
    val recording = feedbackScripted(
      recentPullRequests() answeredWith listAnswer(listedPullRequest(9, "open"), listedPullRequest(8, "closed")),
    )

    val recent = recording.replay { recentPullRequests(PROJECT, SINCE, limit = 1) }.getOrThrow()

    assertEquals(mapOf("9" to true), recent)
  }

  @Test
  fun `a full page of recent pull requests reads the next one`() {
    val full = (200 downTo 101).map { listedPullRequest(it, "open") }
    val recording = feedbackScripted(
      recentPullRequests(page = 1) answeredWith listAnswer(*full.toTypedArray()),
      recentPullRequests(page = 2) answeredWith listAnswer(listedPullRequest(100, "closed")),
    )

    val recent = recording.replay { recentPullRequests(PROJECT, SINCE, limit = 200) }.getOrThrow()

    assertEquals((200 downTo 101).associate { "$it" to true } + ("100" to false), recent)
  }

  @Test
  fun `posts a comment on a line of the pull request's head`() {
    val posted = """{"id": 1, "html_url": "https://github.com/owner/project/pull/7#discussion_r1"}"""
    val recording = feedbackScripted(comment("Is this a problem?") answeredWith answer(201, posted))

    val comment = recording.replay { comment(PULL_REQUEST, HEAD_REVISION, "src/Main.kt", 3, "Is this a problem?") }

    assertEquals(PostedComment.Posted("https://github.com/owner/project/pull/7#discussion_r1"), comment)
  }

  @Test
  fun `a comment GitHub refuses says why`() {
    val refused = """{"message": "Validation Failed", "errors": [{"resource": "PullRequestReviewComment", "code": "invalid"}]}"""
    val recording = feedbackScripted(comment("Is this a problem?") answeredWith answer(422, refused))

    val comment = recording.replay { comment(PULL_REQUEST, HEAD_REVISION, "src/Main.kt", 3, "Is this a problem?") }

    assertEquals(PostedComment.Refused(422, "Validation Failed"), comment)
  }

  @Test
  fun `a comment GitHub refuses for its rate limit says why`() {
    val limited = """{"message": "You have exceeded a secondary rate limit."}"""
    val recording = feedbackScripted(comment("Is this a problem?") answeredWith answer(429, limited))

    val comment = recording.replay { comment(PULL_REQUEST, HEAD_REVISION, "src/Main.kt", 3, "Is this a problem?") }

    assertEquals(PostedComment.Refused(429, "You have exceeded a secondary rate limit."), comment)
  }

  @Test
  fun `a comment GitHub fails to take fails the run`() {
    val recording = feedbackScripted(comment("Is this a problem?") answeredWith answer(502, """{"message": "Server Error"}"""))

    val failure = assertFailsWith<IllegalStateException> {
      recording.replay { comment(PULL_REQUEST, HEAD_REVISION, "src/Main.kt", 3, "Is this a problem?") }
    }

    assertEquals("GitHub answered a review comment on ${PULL_REQUEST.url} with HTTP 502: Server Error", failure.message)
  }
}

/** Since when the fixture routine waits for answers. */
private val SINCE = COLLECTED_AT.minus(FEEDBACK_ANSWER_WINDOW)

/** The request that comments [body] on line 3 of `src/Main.kt` of [PULL_REQUEST]. */
private fun comment(body: String) = ProviderRequest(
  "POST",
  "/repos/owner/project/pulls/7/comments",
  body = buildJsonObject {
    put("body", body)
    put("commit_id", HEAD_REVISION)
    put("path", "src/Main.kt")
    put("line", 3)
    put("side", "RIGHT")
  },
)
