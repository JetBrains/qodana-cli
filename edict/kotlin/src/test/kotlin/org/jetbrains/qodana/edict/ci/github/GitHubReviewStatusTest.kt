// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.github

import org.jetbrains.qodana.edict.ci.MergedReview
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.ci.support.DISCUSSION_6007_START
import org.jetbrains.qodana.edict.ci.support.FORK_REVIEW
import org.jetbrains.qodana.edict.ci.support.MISSING_PULL_REQUEST
import org.jetbrains.qodana.edict.ci.support.MISSING_REVIEW
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_1393
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_5976
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_6007
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_7091
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_7832
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_917
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

  @Test
  fun `a single rebased commit is the merged change, from its parent`() {
    // PR 6007: one commit rebased onto its target, with the discussion of evidence-merged-6007.
    val recording = kotlinRecording("merged-review-6007")

    assertEquals(recording.result<MergedReview>(), recording.replay { mergedReview(PULL_REQUEST_6007) })
  }

  @Test
  fun `messages of apps and of KotlinBuild are left out, while a deleted account's and people's commands are kept`() {
    // PR 6007: one commit rebased onto its target, with the discussion of evidence-merged-6007.
    val recording = kotlinRecording("merged-review-6007")

    assertEquals(DISCUSSION_6007_START, recording.replay { mergedReview(PULL_REQUEST_6007) }.discussionMessages.take(3))
  }

  @Test
  fun `a merge commit is the merged change, from its first parent`() {
    // PR 917: three commits merged by a merge commit.
    val recording = kotlinRecording("merged-review-merge-commit-917")

    assertEquals(recording.result<MergedReview>(), recording.replay { mergedReview(PULL_REQUEST_917) })
  }

  @Test
  fun `a merged change lists the files of every page with their previous paths`() {
    // PR 7091: its merge commit lists 128 files on two pages and renames 7 of them.
    val recording = kotlinRecording("merged-review-paged-renames-7091")

    assertEquals(recording.result<MergedReview>(), recording.replay { mergedReview(PULL_REQUEST_7091) })
  }

  @Test
  fun `several commits rebased onto the target are no merged change`() {
    // PR 7832: four commits rebased onto its target, so no single commit is the change.
    val recording = kotlinRecording("merged-review-rebased-7832")

    val error = assertFailsWith<IllegalArgumentException> { recording.replay { mergedReview(PULL_REQUEST_7832) } }
    assertEquals("Cannot unambiguously identify the complete merged change for this single-parent, multi-commit PR", error.message)
  }

  @Test
  fun `a pull request closed without merging has no merged change`() {
    // PR 5976, closed without merging.
    val recording = kotlinRecording("merged-review-closed-5976")

    val error = assertFailsWith<IllegalArgumentException> { recording.replay { mergedReview(PULL_REQUEST_5976) } }
    assertEquals("Review has not merged", error.message)
  }

  @Test
  fun `the merged change of another repository is rejected before any request`() {
    val error = assertFailsWith<IllegalArgumentException> { kotlinScripted().replay { mergedReview(FORK_REVIEW) } }

    assertEquals(FORK_REJECTION, error.message)
  }
}

private const val FORK_REJECTION =
  "Review https://github.com/fork/kotlin/pull/6007 (github:github.com/fork/kotlin) is not in the configured github:github.com/JetBrains/kotlin"
