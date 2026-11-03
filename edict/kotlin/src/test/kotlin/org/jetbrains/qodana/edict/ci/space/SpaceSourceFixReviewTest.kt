// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.space

import org.jetbrains.qodana.edict.ci.support.BRANCH_CONFLICT
import org.jetbrains.qodana.edict.ci.support.COMMIT_FAILED
import org.jetbrains.qodana.edict.ci.support.RELEASE_191
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_EXISTING_REVIEW
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_NEW_REVIEW
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_SOURCE_FIX_REQUEST
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_UNCHANGED_SOURCE_FIX_REQUEST
import org.jetbrains.qodana.edict.ci.support.UltimateScripts
import org.jetbrains.qodana.edict.ci.support.WRITE_DENIED
import org.jetbrains.qodana.edict.ci.support.spaceEmpty
import org.jetbrains.qodana.edict.ci.support.ultimateRecording
import org.jetbrains.qodana.edict.ci.urlPart
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/**
 * Opening a source-fix review on IJ/ultimate: the regular run reads the real repository until its first write, from
 * which on every answer is scripted; each other test answers one step differently. The review search, creation and reviewer steps it shares with
 * inspection reviews are tested in [SpaceInspectionReviewTest].
 */
internal class SpaceSourceFixReviewTest {
  @Test
  fun `all fixed files are committed at once to a new branch at their base`() {
    // ULTIMATE_SOURCE_FIX_REQUEST opened from a new branch at the real head of RELEASE_191; scripted from the first write on.
    val recording = ultimateRecording("source-fix-review-opened")

    assertEquals(ULTIMATE_NEW_REVIEW, recording.replay { findOrCreateReview(ULTIMATE_SOURCE_FIX_REQUEST) })
  }

  @Test
  fun `a file the fix leaves as it is stops before any write`() {
    // The source fix whose ULTIMATE_BUILD_TXT is the file's real text at its base.
    val recording = ultimateRecording("source-fix-unchanged-file")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_UNCHANGED_SOURCE_FIX_REQUEST) }
    }
    assertEquals("Source fix for 'community/build.txt' has no changes", error.message)
  }

  @Test
  fun `a missing target branch stops before any write`() {
    // source-fix-review-opened, but the target branch doesn't exist.
    val recording = ultimateRecording("source-fix-review-opened")
      .failingAt("GET", "/branches/${urlPart("refs/heads/$RELEASE_191")}", spaceEmpty())

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_SOURCE_FIX_REQUEST) }
    }
    assertEquals("Space target branch does not exist", error.message)
  }

  @Test
  fun `a branch left at the base is committed to without creating it again`() {
    // The source-fix branch exists at the base without the fix: a run stopped before committing.
    val recording = UltimateScripts.sourceFixBranchLeftAtBase()

    assertEquals(ULTIMATE_NEW_REVIEW, recording.replay { findOrCreateReview(ULTIMATE_SOURCE_FIX_REQUEST) })
  }

  @Test
  fun `a branch that already holds the fix is reused with its review`() {
    // The source-fix branch exists at a commit with the fix, from which a review was opened.
    val recording = UltimateScripts.sourceFixAlreadyCommitted()

    assertEquals(ULTIMATE_EXISTING_REVIEW, recording.replay { findOrCreateReview(ULTIMATE_SOURCE_FIX_REQUEST) })
  }

  @Test
  fun `a branch with other content is never overwritten`() {
    // The source-fix branch exists at another commit, whose ULTIMATE_BUILD_TXT isn't the fixed one.
    val recording = UltimateScripts.sourceFixBranchWithOtherContent()

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_SOURCE_FIX_REQUEST) }
    }
    assertEquals("Space source-fix branch already exists with different content", error.message)
  }

  @Test
  fun `a refused branch creation stops before committing, with Space's reason`() {
    // source-fix-review-opened, but Space denies creating the branch.
    val recording = ultimateRecording("source-fix-review-opened").failingAt("POST", "/head", WRITE_DENIED)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_SOURCE_FIX_REQUEST) }
    }
    assertEquals("Space source-fix branch creation returned HTTP 403: Permission is not granted: VcsRepository.Write", error.message)
  }

  @Test
  fun `a refused commit stops before looking for the review`() {
    // source-fix-review-opened, but Space refuses the commit because the branch was updated.
    val recording = ultimateRecording("source-fix-review-opened").failingAt("POST", "/commit", BRANCH_CONFLICT)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_SOURCE_FIX_REQUEST) }
    }
    assertEquals("Space API returned HTTP 409: Branch was updated", error.message)
  }

  @Test
  fun `a commit Space reports failed in a successful response stops before looking for the review`() {
    // source-fix-review-opened, but Space reports the commit as failed in a successful response.
    val recording = ultimateRecording("source-fix-review-opened").failingAt("POST", "/commit", COMMIT_FAILED)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_SOURCE_FIX_REQUEST) }
    }
    assertEquals("Space source-fix commit failed: Branch changed concurrently", error.message)
  }
}
