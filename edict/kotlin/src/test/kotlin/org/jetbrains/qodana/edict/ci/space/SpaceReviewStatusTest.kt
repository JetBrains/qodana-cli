// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.space

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_CLOSED_REVIEW
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_INVISIBLE_REVIEW
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_MERGED_REVIEW
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_OPEN_REVIEW
import org.jetbrains.qodana.edict.ci.support.answer
import org.jetbrains.qodana.edict.ci.support.ultimateRecording
import org.jetbrains.qodana.edict.ci.support.ultimateScripted
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/** The state and discussion of IJ/ultimate reviews, replayed from the real repository. */
internal class SpaceReviewStatusTest {
  @Test
  fun `a review Space closed after merging it is merged`() {
    // ULTIMATE_MERGED_REVIEW, which Space closed after merging it.
    val recording = ultimateRecording("review-merged")

    assertEquals(ReviewStatus(ReviewState.MERGED), recording.replay { review(ULTIMATE_MERGED_REVIEW) })
  }

  @Test
  fun `a review closed without merging is closed unmerged`() {
    // ULTIMATE_CLOSED_REVIEW, closed without merging.
    val recording = ultimateRecording("review-closed")

    assertEquals(ReviewStatus(ReviewState.CLOSED_UNMERGED), recording.replay { review(ULTIMATE_CLOSED_REVIEW) })
  }

  @Test
  fun `an opened review is open`() {
    // ULTIMATE_OPEN_REVIEW, still open.
    val recording = ultimateRecording("review-open")

    assertEquals(ReviewStatus(ReviewState.OPEN), recording.replay { review(ULTIMATE_OPEN_REVIEW) })
  }

  @Test
  fun `a review Space doesn't show is refused`() {
    // ULTIMATE_INVISIBLE_REVIEW, which Space answers with null.
    val recording = ultimateRecording("review-invisible")

    val error = assertFailsWith<IllegalArgumentException> { recording.replay { review(ULTIMATE_INVISIBLE_REVIEW) } }
    assertEquals("Space review id:3CNIEJ2jnTdL is missing or not visible", error.message)
  }

  @Test
  fun `the evidence of a review holds each message of its people once, without events and bots`() {
    // ULTIMATE_MERGED_REVIEW's feed: its discussions' messages, its people's events, and texts and events of the
    // "Patronus" application and of the "JetBrains Space" service.
    val recording = ultimateRecording("evidence-merged")

    assertEquals(recording.result<ReviewStatus>(), recording.replay { reviewWithEvidence(ULTIMATE_MERGED_REVIEW) })
  }

  @Test
  fun `a configured bot's messages are no evidence, whatever the case of its name`() {
    // ULTIMATE_MERGED_REVIEW's feed: its discussions' messages, its people's events, and texts and events of the
    // "Patronus" application and of the "JetBrains Space" service.
    val recording = ultimateRecording("evidence-merged", bots = setOf("lev.leontev"))

    // The evidence starts with three questions of Lev.Leontev, and the answer to them follows.
    assertEquals(
      "I don't think so. It is the same pattern as AbstractGotoSEContributor: the whole fetch runs in one read " +
        "action with write-action priority, so a pending write cancels it. resolveLabel only does VFS/PSI lookups, " +
        "with no refresh. Before this change, resolveLabel ran in a plain runReadAction that blocked writes, so this " +
        "is better for freezes.",
      recording.replay { reviewWithEvidence(ULTIMATE_MERGED_REVIEW) }.evidence.substringBefore("\n\n"),
    )
  }

  @Test
  fun `an application's text messages are no evidence`() {
    // ULTIMATE_OPEN_REVIEW's feed, whose only text messages are the "Patronus" application's.
    val recording = ultimateRecording("evidence-open")

    assertEquals(ReviewStatus(ReviewState.OPEN), recording.replay { reviewWithEvidence(ULTIMATE_OPEN_REVIEW) })
  }

  /** The variant drops `hasMore` from the feed page. */
  @Test
  fun `evidence from an incomplete feed is refused`() {
    // Variant of evidence-open: its feed page omits hasMore.
    val recording = ultimateRecording("evidence-incomplete-feed")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { reviewWithEvidence(ULTIMATE_OPEN_REVIEW) }
    }
    assertEquals(
      "Incomplete Space review feedback: Space review 3mjARa4QCqdV feed page 1: omitted pagination completeness flag",
      error.message,
    )
  }

  @Test
  fun `a review of another repository is refused before any request`() {
    val foreign = ReviewReference(CiProviderId.GITHUB, "JetBrains", "kotlin", "6007", "https://github.com/JetBrains/kotlin/pull/6007")

    val error = assertFailsWith<IllegalArgumentException> { ultimateScripted().replay { review(foreign) } }
    assertEquals(
      "Review https://github.com/JetBrains/kotlin/pull/6007 (github:github.com/JetBrains/kotlin) " +
        "is not in the configured space:jetbrains.team/ij/ultimate",
      error.message,
    )
  }

  @Test
  fun `the change a review merged is unavailable on Space`() {
    val error = assertFailsWith<IllegalStateException> {
      ultimateScripted().replay { mergedReview(ULTIMATE_MERGED_REVIEW) }
    }
    assertEquals("Exact merged-change evidence is unavailable for https://jetbrains.team/p/ij/reviews/224837", error.message)
  }
}
