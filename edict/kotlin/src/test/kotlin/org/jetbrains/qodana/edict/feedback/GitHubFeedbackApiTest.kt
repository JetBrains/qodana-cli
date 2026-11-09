// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.support.ProviderRequest
import org.jetbrains.qodana.edict.ci.support.answer
import org.jetbrains.qodana.edict.ci.support.answeredWith
import org.jetbrains.qodana.edict.feedback.support.HEAD_REVISION
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST_FILES
import org.jetbrains.qodana.edict.feedback.support.feedbackScripted
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
  fun `a comment GitHub fails to take fails the run`() {
    val recording = feedbackScripted(comment("Is this a problem?") answeredWith answer(502, """{"message": "Server Error"}"""))

    val failure = assertFailsWith<IllegalStateException> {
      recording.replay { comment(PULL_REQUEST, HEAD_REVISION, "src/Main.kt", 3, "Is this a problem?") }
    }

    assertEquals("GitHub answered a review comment on ${PULL_REQUEST.url} with HTTP 502: Server Error", failure.message)
  }
}

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
