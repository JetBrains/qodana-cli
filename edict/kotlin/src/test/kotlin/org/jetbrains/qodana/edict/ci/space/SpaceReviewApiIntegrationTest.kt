// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.space

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.DefaultCiHttpTransport
import org.jetbrains.qodana.edict.ci.ReviewFetchResult
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.ci.support.ULTIMATE
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_API
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_CLOSED_REVIEW
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_INVISIBLE_REVIEW
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_MERGED_REVIEW
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_MERGED_REVIEW_BASE
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_OPEN_REVIEW
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_STARLARK_FILE
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_UNCHANGED_SOURCE_FIX_REQUEST
import org.jetbrains.qodana.edict.ci.support.ultimateRecording
import org.junit.jupiter.api.Tag
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable
import java.nio.file.NoSuchFileException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/**
 * The Space client reading the real IJ/ultimate: each test expects what its recording holds, so the recordings
 * the other tests replay stay true. Recordings with scripted answers, such as every write path's, are only replayed
 * mocked: Space can't answer as the script has the repository.
 */
@Tag("integration")
@EnabledIfEnvironmentVariable(named = "SPACE_TOKEN", matches = ".+")
internal class SpaceReviewApiIntegrationTest {
  private val space = DefaultCiHttpTransport(CiProviderId.SPACE, ULTIMATE_API, System.getenv("SPACE_TOKEN"))

  @Test
  fun `a review closed without merging is left out`() {
    // Review 161809, closed without merging.
    val recording = ultimateRecording("fetch-closed-161809")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      recording.replay(space) { fetch(ReviewSelection(1, listOf(161809))) },
    )
  }

  @Test
  fun `a review of commits is reported as no merge request`() {
    // Review 29541: a review of commits, without a branch pair, so no merge request.
    val recording = ultimateRecording("fetch-commit-set-29541")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      recording.replay(space) { fetch(ReviewSelection(1, listOf(29541))) },
    )
  }

  @Test
  fun `an unmerged review of another repository is reported, not left out`() {
    // Review 1946 of the IJ repository ultimate-teamcity-config, closed without merging in 2020.
    val recording = ultimateRecording("fetch-other-repository-1946")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      recording.replay(space) { fetch(ReviewSelection(1, listOf(1946))) },
    )
  }

  @Test
  fun `a code discussion of only an application's messages is left out`() {
    // Review 230785: its only discussion holds one message of the "RubyMine Workflow" application, which opened it.
    val recording = ultimateRecording("fetch-bot-discussion-230785")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      recording.replay(space) { fetch(ReviewSelection(1, listOf(230785))) },
    )
  }

  @Test
  fun `a code discussion an application opened keeps only the people's text messages`() {
    // Review 231618: six discussions "RubyMine Workflow" opened; a person answered and resolved one of them.
    val recording = ultimateRecording("fetch-answered-application-discussion-231618")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      recording.replay(space) { fetch(ReviewSelection(1, listOf(231618))) },
    )
  }

  @Test
  fun `discussions are anchored on their lines, counted from 1, or on the old lines of removed ones`() {
    // Review 232019: `util.kt` is anchored on 0-based line 30, `ProjectViewService.kt` on removed lines, oldLine 8 to 9
    // of `cc94f6cc`, the revision before the change.
    val recording = ultimateRecording("fetch-paged-feed-232019")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      recording.replay(space) { fetch(ReviewSelection(1, listOf(232019))) },
    )
  }

  @Test
  fun `a feed in two pages is read to its end`() {
    // Review 230194: a feed of two pages, whose second page adds the discussions FAIwK0W7A8i and FAKgK0W7AHc; the
    // discussion of `CodeInsightContextManager.kt` spans 0-based lines 90 to 91.
    val recording = ultimateRecording("fetch-multiline-discussion-230194")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      recording.replay(space) { fetch(ReviewSelection(1, listOf(230194))) },
    )
  }

  @Test
  fun `a review with one code discussion has it`() {
    // Review 222772, with one code discussion of one message.
    val recording = ultimateRecording("fetch-one-discussion-222772")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      recording.replay(space) { fetch(ReviewSelection(1, listOf(222772))) },
    )
  }

  /** Only the result: reviews merged since move the range down the discovery pages. */
  @Test
  fun `reviews merged in the date range are found up to the first review merged before it`() {
    // The reviews merged on 2026-11-01, two discovery pages deep, up to the first one merged before.
    val recording = ultimateRecording("fetch-merged-on-2026-11-01")
    val selection = ReviewSelection(100, startDate = "2026-11-01", endDate = "2026-11-01")

    assertEquals(recording.result<ReviewFetchResult>(), SpaceReviewApi(ULTIMATE, space, emptySet()).fetch(selection))
  }

  /** Only the result: reviews merged since move the range down the discovery pages. */
  @Test
  fun `discovery stops at the maximum number of reviews`() {
    // The first two reviews merged on 2026-11-01, with maxPrs 2.
    val recording = ultimateRecording("fetch-first-two-merged-on-2026-11-01")
    val selection = ReviewSelection(2, startDate = "2026-11-01", endDate = "2026-11-01")

    assertEquals(recording.result<ReviewFetchResult>(), SpaceReviewApi(ULTIMATE, space, emptySet()).fetch(selection))
  }

  @Test
  fun `a file is read as it is at a revision`() {
    // ULTIMATE_STARLARK_FILE at ULTIMATE_MERGED_REVIEW_BASE.
    val recording = ultimateRecording("file-at-revision")

    assertEquals(
      recording.result<String>(),
      recording.replay(space) { file(ULTIMATE_MERGED_REVIEW_BASE, ULTIMATE_STARLARK_FILE) },
    )
  }

  @Test
  fun `a path missing at a revision is a missing file`() {
    // A path missing at ULTIMATE_MERGED_REVIEW_BASE.
    val recording = ultimateRecording("file-missing-path")

    val error = assertFailsWith<NoSuchFileException> {
      recording.replay(space) { file(ULTIMATE_MERGED_REVIEW_BASE, "plugins/bazel/Missing.kt") }
    }
    assertEquals("plugins/bazel/Missing.kt", error.message)
  }

  @Test
  fun `a read at a revision Space doesn't have fails rather than finding no file`() {
    // ULTIMATE_STARLARK_FILE at the all-zero revision, which Space doesn't have.
    val recording = ultimateRecording("file-unknown-revision")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay(space) { file("0".repeat(40), ULTIMATE_STARLARK_FILE) }
    }
    assertEquals("Space API returned HTTP 404: Missing unknown 0000000000000000000000000000000000000000", error.message)
  }

  @Test
  fun `without a configured branch reviews target the default branch`() {
    // IJ/ultimate, whose default branch is master.
    val recording = ultimateRecording("default-branch")

    assertEquals("master", recording.replay(space) { resolveTargetBranch(null) })
  }

  @Test
  fun `a review Space closed after merging it is merged`() {
    // ULTIMATE_MERGED_REVIEW, which Space closed after merging it.
    val recording = ultimateRecording("review-merged")

    assertEquals(recording.result<ReviewStatus>(), recording.replay(space) { review(ULTIMATE_MERGED_REVIEW) })
  }

  @Test
  fun `a review closed without merging is closed unmerged`() {
    // ULTIMATE_CLOSED_REVIEW, closed without merging.
    val recording = ultimateRecording("review-closed")

    assertEquals(recording.result<ReviewStatus>(), recording.replay(space) { review(ULTIMATE_CLOSED_REVIEW) })
  }

  @Test
  fun `an opened review is open`() {
    // ULTIMATE_OPEN_REVIEW, still open.
    val recording = ultimateRecording("review-open")

    assertEquals(recording.result<ReviewStatus>(), recording.replay(space) { review(ULTIMATE_OPEN_REVIEW) })
  }

  @Test
  fun `a review Space doesn't show is refused`() {
    // ULTIMATE_INVISIBLE_REVIEW, which Space answers with null.
    val recording = ultimateRecording("review-invisible")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay(space) { review(ULTIMATE_INVISIBLE_REVIEW) }
    }
    assertEquals("Space review id:3CNIEJ2jnTdL is missing or not visible", error.message)
  }

  @Test
  fun `the evidence of a review holds each message of its people once, without events and bots`() {
    // ULTIMATE_MERGED_REVIEW's feed: its discussions' messages, its people's events, and texts and events of the
    // "Patronus" application and of the "JetBrains Space" service.
    val recording = ultimateRecording("evidence-merged")

    assertEquals(
      recording.result<ReviewStatus>(),
      recording.replay(space) { reviewWithEvidence(ULTIMATE_MERGED_REVIEW) },
    )
  }

  @Test
  fun `an application's text messages are no evidence`() {
    // ULTIMATE_OPEN_REVIEW's feed, whose only text messages are the "Patronus" application's.
    val recording = ultimateRecording("evidence-open")

    assertEquals(recording.result<ReviewStatus>(), recording.replay(space) { reviewWithEvidence(ULTIMATE_OPEN_REVIEW) })
  }

  @Test
  fun `a source fix leaving a real file as it is stops before any write`() {
    // The source fix whose ULTIMATE_BUILD_TXT is the file's real text at its base.
    val recording = ultimateRecording("source-fix-unchanged-file")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay(space) { findOrCreateReview(ULTIMATE_UNCHANGED_SOURCE_FIX_REQUEST) }
    }
    assertEquals("Source fix for 'community/build.txt' has no changes", error.message)
  }
}
