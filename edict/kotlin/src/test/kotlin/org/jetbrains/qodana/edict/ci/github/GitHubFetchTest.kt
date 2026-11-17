// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.github

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.ci.ReviewFetchResult
import org.jetbrains.qodana.edict.ci.ReviewMessage
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.ci.feedbackMarker
import org.jetbrains.qodana.edict.ci.support.FIRST_TWO_MERGED_SELECTION
import org.jetbrains.qodana.edict.ci.support.MISSING_PULL_REQUEST
import org.jetbrains.qodana.edict.ci.support.SEVERAL_NUMBERS_SELECTION
import org.jetbrains.qodana.edict.ci.support.TWO_DISCOVERY_PAGES_SELECTION
import org.jetbrains.qodana.edict.ci.support.WEEKEND_SELECTION
import org.jetbrains.qodana.edict.ci.support.kotlinRecording
import org.jetbrains.qodana.edict.ci.support.with
import org.jetbrains.qodana.edict.ci.support.withBody
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.extraction.reviews.withValidThreads
import kotlin.test.Test
import kotlin.test.assertEquals

/** Fetching pull requests of JetBrains/kotlin with their review threads, replaying what GitHub answered. */
internal class GitHubFetchTest {
  @Test
  fun `a merged pull request has its threads, each with its replies`() {
    // PR 6007, merged: a thread of seven messages and one of two, interleaved in time.
    val recording = kotlinRecording("fetch-merged-6007")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay { fetch(selectionOf(6007)) })
  }

  @Test
  fun `a comment on a whole file starts a thread without anchor lines`() {
    // PR 7832: its threads include a comment on a whole file and its reply.
    val recording = kotlinRecording("fetch-file-comments-7832")

    val fetched = recording.replay { fetch(selectionOf(7832)) }

    val thread = fetched.pullRequests.single().threads.single { it.threadId == FILE_THREAD }

    assertEquals(null to null, thread.anchorLine to thread.anchorEndLine)
  }

  @Test
  fun `threads on whole files pass the thread validation`() {
    // PR 7832: its threads include a comment on a whole file and its reply.
    val recording = kotlinRecording("fetch-file-comments-7832")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      recording.replay { fetch(selectionOf(7832)) }.withValidThreads(),
    )
  }

  @Test
  fun `a pull request closed without merging is left out without a problem`() {
    // PR 5976, closed without merging.
    val recording = kotlinRecording("fetch-closed-5976")

    assertEquals(ReviewFetchResult(emptyList()), recording.replay { fetch(selectionOf(5976)) })
  }

  @Test
  fun `a missing pull request is a problem that names what GitHub doesn't have`() {
    // A pull request number GitHub doesn't have.
    val recording = kotlinRecording("fetch-missing-$MISSING_PULL_REQUEST")

    val fetched = recording.replay { fetch(selectionOf(MISSING_PULL_REQUEST)) }

    assertEquals(ReviewFetchResult(emptyList(), listOf(MISSING_PROBLEM)), fetched)
  }

  @Test
  fun `pull requests are fetched in the requested order, past missing and unmerged ones`() {
    // SEVERAL_NUMBERS_SELECTION: three merged PRs in neither number order, a missing one and one closed unmerged.
    val recording = kotlinRecording("fetch-several-numbers")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay { fetch(SEVERAL_NUMBERS_SELECTION) })
  }

  @Test
  fun `a review bot's thread is kept, its messages marked as from a bot`() {
    // PR 7185: a thread Copilot started in a review whose body Copilot wrote, nobody answering, and a thread of people.
    val recording = kotlinRecording("fetch-bot-thread-7185")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay { fetch(selectionOf(7185)) })
  }

  @Test
  fun `a configured bot's comments are left out of threads, whatever the case of its login`() {
    // PR 7185: timofeys1 opened the thread of people, which rbbozkurt answered; Copilot alone wrote the other thread.
    val bots = setOf("TimofeyS1", "COPILOT-PULL-REQUEST-REVIEWER[BOT]", "copilot")
    val recording = kotlinRecording("fetch-bot-thread-7185", bots = bots)

    val threads = recording.replay { fetch(selectionOf(7185)) }.pullRequests.single().threads

    assertEquals(listOf("github-7185-3721437321" to listOf("rbbozkurt")), threads.map { it.threadId to it.messages.map(ReviewMessage::author) })
  }

  @Test
  fun `a thread with Edict's own feedback comment is left out, whoever posted it`() {
    // PR 7185, with the people's thread opened by an Edict finding that timofeys1 posted and rbbozkurt answered.
    val recording = kotlinRecording("fetch-bot-thread-7185").answering("GET", "/pulls/7185/comments") { comments ->
      val bodies = comments.response.body.jsonArray.map { comment ->
        if (comment.jsonObject.number("id") != 3721437321) comment
        else comment.with("body" to JsonPrimitive("Edict test finding\n\n${feedbackMarker("f-0123456789abcdef01234567")}"))
      }
      listOf(comments.withBody(JsonArray(bodies)))
    }

    val threads = recording.replay { fetch(selectionOf(7185)) }.pullRequests.single().threads

    assertEquals(listOf("github-7185-3701804217"), threads.map { it.threadId })
  }

  @Test
  fun `review comments on two pages make complete threads, each opened by the body of its review`() {
    // PR 6231: 131 review comments on two pages, and reviews with bodies.
    val recording = kotlinRecording("fetch-paged-comments-6231")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay { fetch(selectionOf(6231)) })
  }

  @Test
  fun `pull requests merged in a date range are discovered until one was updated before the range`() {
    // WEEKEND_SELECTION: discovery stops at the first PR updated before the weekend.
    val recording = kotlinRecording("fetch-merged-on-weekend")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay { fetch(WEEKEND_SELECTION) })
  }

  @Test
  fun `discovery stops at the maximum number of pull requests`() {
    // FIRST_TWO_MERGED_SELECTION: discovery stops after 9417, the second PR merged in the range.
    val recording = kotlinRecording("fetch-first-two-merged")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay { fetch(FIRST_TWO_MERGED_SELECTION) })
  }

  @Test
  fun `discovery reads the next page while the first lists only pull requests updated after the range`() {
    // TWO_DISCOVERY_PAGES_SELECTION: the first page lists only pull requests updated after 2026-10-28.
    val recording = kotlinRecording("fetch-two-discovery-pages")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay { fetch(TWO_DISCOVERY_PAGES_SELECTION) })
  }

  @Test
  fun `a pull request listed again on the next discovery page is fetched once and reported`() {
    // TWO_DISCOVERY_PAGES_SELECTION, with the second page read after a pull request was updated meanwhile: that moves
    // the first page's last pull request onto the second.
    val recording = kotlinRecording("fetch-two-discovery-pages")
    val last = recording.exchanges.first().response.body.jsonArray.last()
    val moved = recording.answering("GET", "/pulls", mapOf("page" to "2")) { page ->
      listOf(page.withBody(JsonArray(listOf(last) + page.response.body.jsonArray.dropLast(1))))
    }

    val expected = recording.result<ReviewFetchResult>()
      .let { it.copy(problems = listOf("GitHub PR discovery page 2 item[0]: invalid or repeated PR number") + it.problems) }
    assertEquals(expected, moved.replay { fetch(TWO_DISCOVERY_PAGES_SELECTION) })
  }
}

/** Why the fetch leaves out [MISSING_PULL_REQUEST]. */
private const val MISSING_PROBLEM = "GitHub PR 999999: GitHub read of /repos/JetBrains/kotlin/pulls/999999 returned HTTP 404: Not Found"

/** The thread of pull request 7832 on the whole of `KaScopeProvider.kt`. */
private const val FILE_THREAD = "github-7832-3904783992"

private fun selectionOf(number: Int) = ReviewSelection(maxPrs = 1, prNumbers = listOf(number))
