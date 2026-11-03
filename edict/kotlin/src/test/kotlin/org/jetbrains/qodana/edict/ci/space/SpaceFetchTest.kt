// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.space

import org.jetbrains.qodana.edict.ci.ReviewFetchResult
import org.jetbrains.qodana.edict.ci.ReviewMessage
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.ci.ReviewThread
import org.jetbrains.qodana.edict.ci.support.UltimateScripts
import org.jetbrains.qodana.edict.ci.support.ultimateRecording
import org.jetbrains.qodana.edict.extraction.reviews.withValidThreads
import kotlin.test.Test
import kotlin.test.assertEquals

/** Fetching reviews of IJ/ultimate by number and by merge date, replayed from the real repository. */
internal class SpaceFetchTest {
  @Test
  fun `a review closed without merging is left out`() {
    // Review 161809, closed without merging.
    val recording = ultimateRecording("fetch-closed-161809")

    assertEquals(ReviewFetchResult(emptyList()), recording.replay { fetch(ReviewSelection(1, listOf(161809))) })
  }

  @Test
  fun `a review of commits is reported as no merge request`() {
    // Review 29541: a review of commits, without a branch pair, so no merge request.
    val recording = ultimateRecording("fetch-commit-set-29541")

    val fetched = recording.replay { fetch(ReviewSelection(1, listOf(29541))) }

    assertEquals(
      ReviewFetchResult(emptyList(), listOf("Space PR 29541: Space review is a review of commits, not a merge request")),
      fetched,
    )
  }

  @Test
  fun `an unmerged review of another repository is reported, not left out`() {
    // Review 1946 of the IJ repository ultimate-teamcity-config, closed without merging in 2020.
    val recording = ultimateRecording("fetch-other-repository-1946")

    val fetched = recording.replay { fetch(ReviewSelection(1, listOf(1946))) }

    assertEquals(ReviewFetchResult(emptyList(), listOf("Space PR 1946: Space review of another repository")), fetched)
  }

  @Test
  fun `a code discussion of only an application's messages is left out`() {
    // Review 230785: its only discussion holds one message of the "RubyMine Workflow" application, which opened it.
    val recording = ultimateRecording("fetch-bot-discussion-230785")

    val fetched = recording.replay { fetch(ReviewSelection(1, listOf(230785))) }

    assertEquals(recording.result<ReviewFetchResult>(), fetched)
    assertEquals(emptyList(), fetched.pullRequests.single().threads)
  }

  @Test
  fun `a feed in two pages is read to its end`() {
    // Review 230194: a feed of two pages, whose second page adds the discussions FAIwK0W7A8i and FAKgK0W7AHc.
    val recording = ultimateRecording("fetch-multiline-discussion-230194")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay { fetch(ReviewSelection(1, listOf(230194))) })
  }

  @Test
  fun `a discussion without an end anchor is on one line, counted from 1`() {
    // Review 232019: a feed of two pages; `util.kt` is anchored on 0-based line 30, `ProjectViewService.kt` on
    // removed lines, oldLine 8 to 9 of `cc94f6cc`, the revision before the change.
    val recording = ultimateRecording("fetch-paged-feed-232019")

    val thread = recording.replay { fetch(ReviewSelection(1, listOf(232019))) }.thread("/util.kt")

    assertEquals(Triple("aec19176ea30846c32d87a3b5fa43f63fa24d940", 31, 31), thread.anchor())
  }

  @Test
  fun `a discussion spans its lines to its end anchor`() {
    // Review 230194: the discussion of `CodeInsightContextManager.kt` spans 0-based lines 90 to 91.
    val recording = ultimateRecording("fetch-multiline-discussion-230194")

    val thread = recording.replay { fetch(ReviewSelection(1, listOf(230194))) }.thread("/CodeInsightContextManager.kt")

    assertEquals(Triple("028ba0bca14c251901fb9bfc2cdb148d94cf277c", 91, 92), thread.anchor())
  }

  @Test
  fun `a discussion on removed lines is anchored on their old lines in the revision before the change`() {
    // Review 232019: a feed of two pages; `util.kt` is anchored on 0-based line 30, `ProjectViewService.kt` on
    // removed lines, oldLine 8 to 9 of `cc94f6cc`, the revision before the change.
    val recording = ultimateRecording("fetch-paged-feed-232019")

    val thread = recording.replay { fetch(ReviewSelection(1, listOf(232019))) }.thread("/ProjectViewService.kt")

    assertEquals(Triple("cc94f6cc07602e6066a2d6d11fdfb9e9ffd1365a", 9, 10), thread.anchor())
  }

  @Test
  fun `a code discussion an application opened keeps only the people's text messages`() {
    // Review 231618: six discussions "RubyMine Workflow" opened; a person answered and resolved one of them.
    val recording = ultimateRecording("fetch-answered-application-discussion-231618")

    val fetched = recording.replay { fetch(ReviewSelection(1, listOf(231618))) }

    assertEquals(recording.result<ReviewFetchResult>(), fetched)
    assertEquals(listOf(listOf("Henry.Wylde")), fetched.pullRequests.single().threads.map { it.messages.map(ReviewMessage::author) })
  }

  @Test
  fun `a configured bot's messages are left out of discussions, whatever the case of its name`() {
    // Review 231618: six discussions "RubyMine Workflow" opened; only Henry.Wylde answered one of them.
    val recording = ultimateRecording("fetch-answered-application-discussion-231618", bots = setOf("henry.wylde"))

    assertEquals(emptyList(), recording.replay { fetch(ReviewSelection(1, listOf(231618))) }.pullRequests.single().threads)
  }

  @Test
  fun `a discussion in two pages is read to its end`() {
    // Review 222772, with its discussion's one message repeated 51 times, in a page of 50 and a page of 1 (scripted).
    val recording = UltimateScripts.discussionInTwoPages()

    val expected = oneDiscussion().withThreads { thread -> listOf(thread.copy(messages = List(51) { thread.messages.single() })) }
    assertEquals(expected, recording.replay { fetch(REVIEW_222772) })
  }

  @Test
  fun `a discussion on a whole file has no anchor lines`() {
    // Review 222772, with its discussion anchored on its file, without lines (scripted).
    val recording = UltimateScripts.wholeFileDiscussion()

    val expected = oneDiscussion().withThreads { thread -> listOf(thread.copy(anchorLine = null, anchorEndLine = null)) }
    assertEquals(expected, recording.replay { fetch(REVIEW_222772) })
  }

  @Test
  fun `a discussion whose history Space limits is reported and left out`() {
    // Review 222772, with Space saying its discussion's history is limited (scripted).
    val recording = UltimateScripts.limitedDiscussionHistory()

    val expected = oneDiscussion().withThreads { emptyList() }
      .copy(problems = listOf("Space PR 222772 discussion 20NyBn21e5p3 page 1: history unavailable or limited"))
    assertEquals(expected, recording.replay { fetch(REVIEW_222772) })
  }

  @Test
  fun `a feed whose cursor doesn't advance is reported and its messages kept`() {
    // Review 222772, with its feed saying more follow, under the cursor it was read with (scripted).
    val recording = UltimateScripts.stalledFeed()

    val expected = oneDiscussion().copy(problems = listOf("Space PR 222772 feed page 1: pagination did not advance"))
    assertEquals(expected, recording.replay { fetch(REVIEW_222772) })
  }

  @Test
  fun `a malformed feed element is reported and the review kept`() {
    // Review 222772, with the first element of its feed, a review event, null (scripted).
    val recording = UltimateScripts.malformedFeedElement()

    val expected = oneDiscussion().copy(problems = listOf("Space PR 222772 feed page 1 data[0]: expected object"))
    assertEquals(expected, recording.replay { fetch(REVIEW_222772) })
  }

  @Test
  fun `reviews merged in the date range are found up to the first review merged before it`() {
    // The reviews merged on 2026-11-01, two discovery pages deep, up to the first one merged before.
    val recording = ultimateRecording("fetch-merged-on-2026-11-01")

    val fetched = recording.replay { fetch(ReviewSelection(100, startDate = "2026-11-01", endDate = "2026-11-01")) }

    assertEquals(recording.result<ReviewFetchResult>(), fetched)
  }

  @Test
  fun `discovery stops at the maximum number of reviews`() {
    // The first two reviews merged on 2026-11-01, with maxPrs 2.
    val recording = ultimateRecording("fetch-first-two-merged-on-2026-11-01")

    val fetched = recording.replay { fetch(ReviewSelection(2, startDate = "2026-11-01", endDate = "2026-11-01")) }

    assertEquals(recording.result<ReviewFetchResult>(), fetched)
    assertEquals(listOf(230194, 232745), fetched.pullRequests.map { it.number })
  }

  @Test
  fun `malformed discovery elements are reported and skipped`() {
    // The first two reviews merged on 2026-11-01, with null, a reviewless element and a repeat first on the page.
    val recording = UltimateScripts.malformedDiscovery()

    // The first two reviews merged on 2026-11-01, as recorded.
    val expected = ultimateRecording("fetch-first-two-merged-on-2026-11-01").result<ReviewFetchResult>().copy(
      problems = listOf(
        "Space review discovery page 1 data[0]: expected object",
        "Space review discovery page 1 data[1]: missing review object",
        "Space review discovery page 1 data[3]: invalid or repeated review",
      ),
    )
    assertEquals(expected, recording.replay { fetch(ReviewSelection(2, startDate = "2026-11-01", endDate = "2026-11-01")) })
  }

  @Test
  fun `fetched reviews have exact revisions and valid anchors`() {
    // The reviews merged on 2026-11-01, two discovery pages deep, up to the first one merged before.
    val recording = ultimateRecording("fetch-merged-on-2026-11-01")

    val fetched = recording.replay { fetch(ReviewSelection(100, startDate = "2026-11-01", endDate = "2026-11-01")) }

    assertEquals(fetched, fetched.withValidThreads())
  }
}

/** The only thread of the only fetched review on a file whose path ends with [pathSuffix]. */
private fun ReviewFetchResult.thread(pathSuffix: String): ReviewThread =
  pullRequests.single().threads.single { it.filePath.endsWith(pathSuffix) }

/** The revision and the first and last line a thread is anchored on. */
private fun ReviewThread.anchor(): Triple<String, Int?, Int?> = Triple(originalCommitSha, anchorLine, anchorEndLine)

private val REVIEW_222772 = ReviewSelection(1, listOf(222772))

/** What review 222772 fetches as recorded: one code discussion with one person's message. */
private fun oneDiscussion(): ReviewFetchResult = ultimateRecording("fetch-one-discussion-222772").result()

/** This result with each review's threads replaced by what [change] makes of each. */
private fun ReviewFetchResult.withThreads(change: (ReviewThread) -> List<ReviewThread>): ReviewFetchResult =
  copy(pullRequests = pullRequests.map { it.copy(threads = it.threads.flatMap(change)) })
