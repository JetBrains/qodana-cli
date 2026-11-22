// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.github

import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.ci.support.FORK_REVIEW
import org.jetbrains.qodana.edict.ci.support.MISSING_PULL_REQUEST
import org.jetbrains.qodana.edict.ci.support.MISSING_REVIEW
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_1393
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_5976
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_6007
import org.jetbrains.qodana.edict.ci.support.kotlinRecording
import org.jetbrains.qodana.edict.ci.support.kotlinScripted
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/** The state, discussion and merged change of reviews of JetBrains/kotlin, replaying what GitHub answered. */
internal class GitHubReviewStatusTest {
  @Test
  fun `a pull request merged on GitHub is merged although GitHub closed it`() {
    // PR 6007, merged: GitHub reports it closed, with merged_at.
    val recording = kotlinRecording("review-merged-6007")

    assertEquals(ReviewStatus(ReviewState.MERGED), recording.replay { review(PULL_REQUEST_6007) })
  }

  @Test
  fun `a pull request closed without merging is closed unmerged`() {
    // PR 5976, closed without merging.
    val recording = kotlinRecording("review-closed-5976")

    assertEquals(ReviewStatus(ReviewState.CLOSED_UNMERGED), recording.replay { review(PULL_REQUEST_5976) })
  }

  @Test
  fun `an open pull request is open`() {
    // PR 1393, still open.
    val recording = kotlinRecording("review-open-1393")

    assertEquals(ReviewStatus(ReviewState.OPEN), recording.replay { review(PULL_REQUEST_1393) })
  }

  @Test
  fun `a missing pull request fails`() {
    // A pull request number GitHub doesn't have.
    val recording = kotlinRecording("review-missing-$MISSING_PULL_REQUEST")

    val error = assertFailsWith<IllegalArgumentException> { recording.replay { review(MISSING_REVIEW) } }
    assertEquals("GitHub pull-request read returned HTTP 404: Not Found", error.message)
  }

  @Test
  fun `a review of another repository is rejected before any request`() {
    val error = assertFailsWith<IllegalArgumentException> { kotlinScripted().replay { review(FORK_REVIEW) } }

    assertEquals(FORK_REJECTION, error.message)
  }

  @Test
  fun `evidence is the discussion, its reviews and their comments without the bots`() {
    // PR 6007's discussion: people, the deleted `ghost`, kotlin-safemerge[bot] and the user KotlinBuild.
    val recording = kotlinRecording("evidence-merged-6007")

    assertEquals(recording.result<ReviewStatus>(), recording.replay { reviewWithEvidence(PULL_REQUEST_6007) })
  }

  @Test
  fun `a pull request closed without merging has its evidence`() {
    // PR 5976, closed without merging, with its discussion.
    val recording = kotlinRecording("evidence-closed-5976")

    assertEquals(recording.result<ReviewStatus>(), recording.replay { reviewWithEvidence(PULL_REQUEST_5976) })
  }

  @Test
  fun `an open pull request has its evidence`() {
    // PR 1393, open, with its discussion.
    val recording = kotlinRecording("evidence-open-1393")

    assertEquals(recording.result<ReviewStatus>(), recording.replay { reviewWithEvidence(PULL_REQUEST_1393) })
  }
}

private const val FORK_REJECTION =
  "Review https://github.com/fork/kotlin/pull/6007 (github:github.com/fork/kotlin) is not in the configured github:github.com/JetBrains/kotlin"
